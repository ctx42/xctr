// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/ctx42/gitaid/pkg/gitaid"
	"github.com/ctx42/ring/pkg/ring"
	"github.com/ctx42/xdef/pkg/xdef"
	"github.com/distribution/reference"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	tc "github.com/testcontainers/testcontainers-go"
)

// SetEnvCTRLog sets [EnvCTRLog] to "true" in the environment.
func SetEnvCTRLog(env []string) []string {
	return ring.EnvSet(env, EnvCTRLog, "true")
}

// SetEnvCTRBuildLog sets [EnvCTRBuildLog] to "true" in the environment.
func SetEnvCTRBuildLog(env []string) []string {
	return ring.EnvSet(env, EnvCTRBuildLog, "true")
}

// SetEnvCTREntrypoint sets [EnvCTREntrypoint] in environment to given command.
// When Command is not provided the default is used: tini -- tail -f /dev/null.
//
// NOTE: It does not support quoted strings.
func SetEnvCTREntrypoint(env []string, cmd ...string) []string {
	if len(cmd) == 0 {
		cmd = []string{"tini", "--", "tail", "-f", "/dev/null"}
	}
	return ring.EnvSet(env, EnvCTREntrypoint, strings.Join(cmd, " "))
}

// newDockerClient creates a Docker API client for the same Docker host
// testcontainers uses to start containers: the Docker context, the
// testcontainers properties file, or DOCKER_HOST. Tests may replace it to
// force client-creation failures.
var newDockerClient = func(ctx context.Context) (*client.Client, error) {
	cli, err := tc.NewDockerClientWithOpts(ctx)
	if err != nil {
		return nil, err
	}
	return cli.Client, nil
}

// Option is an option for [NewCTR].
type Option func(*CTR)

// WithCTRImgRm is an option for [NewCTR] that removes the container image in
// [CTR.Terminate].
func WithCTRImgRm(ctr *CTR) { ctr.removeImage = true }

// WithCTRLogger is an option for [NewCTR] setting a custom log consumer. The
// consumer is attached whether or not [EnvCTRLog] is set, unless the request
// already has a log consumer config.
func WithCTRLogger(lc tc.LogConsumer) Option {
	return func(ctr *CTR) { ctr.log = lc }
}

type hidCleanTrait = CleanupTrait // Hide embedded field.

// CTR represents a Docker container.
type CTR struct {
	hidCleanTrait

	// Unique container ID. Set by [CTR.Start].
	id string

	// Human-readable container name, usually set to test name which created it.
	//
	// It's set as the value for [LabTestCtrName] label.
	name string

	// Image reference.
	//
	// Examples: "repo/name:tag", "ealen/echo-server:0.8.12".
	reference string

	// Container request as configured by the caller.
	req tc.GenericContainerRequest

	// Request the last [CTR.Start] passed to testcontainers: req with
	// provenance labels and env, build arguments, and env switches applied.
	runReq tc.GenericContainerRequest

	// Started container. Set by [CTR.Start] and set to nil by
	// [CTR.Terminate].
	dc *tc.DockerContainer

	// When true and the container request is based on image it will be removed
	// in [CTR.Terminate].
	removeImage bool

	// Container configuration as seen from the host perspective.
	cfgHost map[string]string

	// Container configuration as seen from the guest perspective.
	cfgGuest map[string]string

	// Set to true when [CTR.Terminate] was called.
	terminated bool

	// When true only read-only operations on the container are allowed.
	readOnly bool

	// Exec whitelist.
	wl *Whitelist

	// Docker client connection.
	//
	// Used to execute commands on started container.
	cli *client.Client

	// Guards cli against concurrent Exec and Terminate calls.
	cliMx sync.Mutex

	// Custom container log consumer.
	log tc.LogConsumer
}

var (
	_ Container  = (*CTR)(nil)
	_ Descriptor = (*CTR)(nil)
)

// NewCTR returns an instance of [CTR] for the given request. The request's
// labels, environment, build arguments, exposed ports, and files are copied,
// so the same request may be used to create many containers.
func NewCTR(name string, req tc.GenericContainerRequest, opts ...Option) *CTR {
	ctr := &CTR{name: name, req: cloneReq(req)}
	for _, opt := range opts {
		opt(ctr)
	}
	return ctr
}

// cloneReq returns a copy of the request whose maps and slices that [CTR]
// writes to are not shared with the original.
func cloneReq(req tc.GenericContainerRequest) tc.GenericContainerRequest {
	req.Labels = maps.Clone(req.Labels)
	req.Env = maps.Clone(req.Env)
	req.BuildArgs = maps.Clone(req.BuildArgs)
	req.ExposedPorts = slices.Clone(req.ExposedPorts)
	req.Files = slices.Clone(req.Files)
	return req
}

func (ctr *CTR) ID() string        { return ctr.id }
func (ctr *CTR) Name() string      { return ctr.name }
func (ctr *CTR) Reference() string { return ctr.reference }
func (ctr *CTR) IsReadOnly() bool  { return ctr.readOnly }

func (ctr *CTR) IsRunning() bool {
	return ctr.dc != nil && ctr.dc.IsRunning()
}

func (ctr *CTR) ConfigHost() map[string]string {
	return maps.Clone(ctr.cfgHost)
}

func (ctr *CTR) ConfigGuest() map[string]string {
	return maps.Clone(ctr.cfgGuest)
}

func (ctr *CTR) SetReadOnly(_ context.Context, expr ...string) error {
	if err := ctr.SetExecWhitelist(expr...); err != nil {
		return err
	}
	ctr.readOnly = true
	return nil
}

func (ctr *CTR) SetExecWhitelist(expr ...string) error {
	if ctr.wl != nil {
		return errors.New("whitelist already set")
	}
	wl := NewWhitelist()
	if err := wl.Add(expr...); err != nil {
		return err
	}
	ctr.wl = wl
	return nil
}

// CreateTemp creates a temporary file on the host with the pattern (see
// [os.CreateTemp]) and given content. Returns absolute path to the created
// file. The file will be removed during cleanup.
func (ctr *CTR) CreateTemp(pattern string, content []byte) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("create temp: %w", err)
	}
	path := f.Name()
	defer func() { _ = f.Close() }()
	if _, err = f.Write(content); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("write temp: %w", err)
	}
	ctr.RegisterCleanup(func(context.Context) error { return os.Remove(path) })
	// The file is world-readable, so processes inside the container can read
	// it.
	if err = os.Chmod(path, 0644); err != nil { //nolint:gosec
		return "", fmt.Errorf("chmod temp: %w", err)
	}
	return path, nil
}

func (ctr *CTR) Exec(ctx context.Context, cmd ...string) ExecResult {
	if len(cmd) == 0 {
		return ExecResult{ExecError: ErrEmptyCmd}
	}
	if ctr.dc == nil {
		return ExecResult{ExecError: ErrNotStarted}
	}
	if ctr.wl != nil {
		if err := ctr.wl.Check(cmd...); err != nil {
			return ExecResult{ExecError: err}
		}
	}

	ctr.cliMx.Lock()
	if ctr.cli == nil {
		var err error
		if ctr.cli, err = newDockerClient(ctx); err != nil {
			ctr.cliMx.Unlock()
			return ExecResult{ExecError: fmt.Errorf("docker client: %w", err)}
		}
	}
	cli := ctr.cli
	ctr.cliMx.Unlock()

	// Prepare exec.
	cfgCrt := client.ExecCreateOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}
	rspCrt, err := cli.ExecCreate(ctx, ctr.ID(), cfgCrt)
	if err != nil {
		return ExecResult{ExecError: fmt.Errorf("exec create: %w", err)}
	}

	// Run command with stdout and stderr attached.
	cfgAtt := client.ExecAttachOptions{}
	rspAtt, err := cli.ExecAttach(ctx, rspCrt.ID, cfgAtt)
	if err != nil {
		return ExecResult{ExecError: fmt.Errorf("exec attach: %w", err)}
	}
	defer rspAtt.Close()

	// Read the output.
	var outBuf, errBuf bytes.Buffer
	outputDone := make(chan error, 1)

	go func() {
		// StdCopy de-multiplexes the stream into two buffers.
		_, copyErr := stdcopy.StdCopy(&outBuf, &errBuf, rspAtt.Reader)
		outputDone <- copyErr
	}()

	select {
	case err = <-outputDone:
		if err != nil {
			return ExecResult{ExecError: fmt.Errorf("exec output: %w", err)}
		}

	case <-ctx.Done():
		// Close the "attach" stream so StdCopy unblocks, then wait for the
		// goroutine so it does not race on the buffers after return.
		rspAtt.Close()
		<-outputDone
		return ExecResult{ExecError: ctx.Err()}
	}

	// Get the exit code.
	cfgIns := client.ExecInspectOptions{}
	irsp, err := cli.ExecInspect(ctx, rspCrt.ID, cfgIns)
	if err != nil {
		return ExecResult{ExecError: fmt.Errorf("exec inspect: %w", err)}
	}

	return ExecResult{
		ExitCode: irsp.ExitCode,
		SOut:     outBuf.String(),
		EOut:     errBuf.String(),
	}
}

// ExecContent uploads content as a uniquely named file to the running
// container's /tmp directory and executes it. The file stays in the container.
// The container must not be a read-only container.
func (ctr *CTR) ExecContent(ctx context.Context, content []byte) ExecResult {
	if ctr.readOnly {
		return ExecResult{ExecError: ErrReadOnly}
	}
	// The container path uses forward slashes on every host OS.
	dst := path.Join("/tmp", "xctr-"+rand.Text()+".sh")
	if err := ctr.CopyTo(ctx, bytes.NewReader(content), dst, 0755); err != nil {
		return ExecResult{ExecError: err}
	}
	return ctr.Exec(ctx, dst)
}

// ExecFile uploads the file at path to the running container and executes it.
// The container must not be a read-only container.
func (ctr *CTR) ExecFile(ctx context.Context, pth string) ExecResult {
	// The path is a caller-supplied file to upload and run in the container.
	content, err := os.ReadFile(pth) //nolint:gosec
	if err != nil {
		return ExecResult{ExecError: fmt.Errorf("exec file: %w", err)}
	}
	return ctr.ExecContent(ctx, content)
}

// AddFile adds files on the host system to the container. It must be called
// before the container is started; otherwise it returns [ErrRunning].
func (ctr *CTR) AddFile(files ...tc.ContainerFile) error {
	if ctr.IsRunning() {
		return ErrRunning
	}
	ctr.req.Files = append(ctr.req.Files, files...)
	return nil
}

func (ctr *CTR) CopyTo(
	ctx context.Context,
	rdr io.Reader,
	dst string,
	mode int64,
) error {

	if ctr.dc == nil {
		return ErrNotStarted
	}
	if ctr.readOnly {
		return ErrReadOnly
	}

	data, err := io.ReadAll(rdr)
	if err != nil {
		return fmt.Errorf("copy to %s: %w", dst, err)
	}
	if err = ctr.dc.CopyToContainer(ctx, data, dst, mode); err != nil {
		return fmt.Errorf("copy to %s: %w", dst, err)
	}
	return nil
}

func (ctr *CTR) ReadFile(ctx context.Context, pth string) ([]byte, error) {
	if ctr.dc == nil {
		return nil, ErrNotStarted
	}
	rc, err := ctr.dc.CopyFileFromContainer(ctx, pth)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", pth, err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", pth, err)
	}
	return data, nil
}

func (ctr *CTR) Setenv(key, value string) error {
	if ctr.IsRunning() {
		return ErrRunning
	}
	if ctr.req.Env == nil {
		ctr.req.Env = make(map[string]string)
	}
	ctr.req.Env[key] = value
	return nil
}

func (ctr *CTR) SetLabel(name, value string) error {
	if ctr.IsRunning() {
		return ErrRunning
	}
	if ctr.req.ShouldBuildImage() {
		orig := ctr.req.BuildOptionsModifier
		ctr.req.BuildOptionsModifier = func(opts *client.ImageBuildOptions) {
			if orig != nil {
				orig(opts)
			}
			if opts.Labels == nil {
				opts.Labels = make(map[string]string)
			}
			opts.Labels[name] = value
		}
		return nil
	}
	if ctr.req.Labels == nil {
		ctr.req.Labels = make(map[string]string)
	}
	ctr.req.Labels[name] = value
	return nil
}

func (ctr *CTR) Request() tc.GenericContainerRequest { return ctr.req }

// Container returns the started container or nil.
func (ctr *CTR) Container() *tc.DockerContainer { return ctr.dc }

// ExposePort exposes port on a container. Must be set before the container is
// started.
func (ctr *CTR) ExposePort(p string) error {
	if ctr.IsRunning() {
		return ErrRunning
	}
	ctr.req.ExposedPorts = append(ctr.req.ExposedPorts, p)
	return nil
}

// MappedPort returns the externally mapped port for the given container port.
// Returns [ErrNotStarted] when the container is not running.
func (ctr *CTR) MappedPort(
	ctx context.Context,
	p network.Port,
) (network.Port, error) {

	if ctr.dc == nil {
		return network.Port{}, ErrNotStarted
	}
	port, err := ctr.dc.MappedPort(ctx, p.String())
	if err != nil {
		return network.Port{}, fmt.Errorf("mapped port %s: %w", p, err)
	}
	return port, nil
}

// ContainerIP returns the primary container IP. Returns [ErrNotStarted] when
// the container is not running.
func (ctr *CTR) ContainerIP(ctx context.Context) (string, error) {
	if ctr.dc == nil {
		return "", ErrNotStarted
	}
	ip, err := ctr.dc.ContainerIP(ctx)
	if err != nil {
		return "", fmt.Errorf("container ip: %w", err)
	}
	return ip, nil
}

// GatewayIP returns the gateway IP of the network connected to the container
// whose name sorts first. Returns [ErrNoNetwork] when no network is connected,
// [ErrNotStarted] when the container is not running, and an error when that
// network has no gateway.
func (ctr *CTR) GatewayIP(ctx context.Context) (string, error) {
	if ctr.dc == nil {
		return "", ErrNotStarted
	}
	nc, err := ctr.dc.Inspect(ctx)
	if err != nil {
		return "", fmt.Errorf("gateway ip: %w", err)
	}
	nets := nc.NetworkSettings.Networks
	if len(nets) == 0 {
		return "", ErrNoNetwork
	}
	first := slices.Min(slices.Collect(maps.Keys(nets)))
	gw := nets[first].Gateway
	if !gw.IsValid() {
		return "", fmt.Errorf("gateway ip: network %s has no gateway", first)
	}
	return gw.String(), nil
}

// Pause pauses running container.
func (ctr *CTR) Pause(ctx context.Context) error {
	if ctr.dc == nil {
		return ErrNotStarted
	}
	p, err := tc.NewDockerProvider()
	if err != nil {
		return fmt.Errorf("pause: %w", err)
	}
	defer func() { _ = p.Close() }()
	cli := p.Client()
	_, err = cli.ContainerPause(ctx, ctr.id, client.ContainerPauseOptions{})
	if err != nil {
		return fmt.Errorf("pause: %w", err)
	}
	return nil
}

// Unpause unpauses running container.
func (ctr *CTR) Unpause(ctx context.Context) error {
	if ctr.dc == nil {
		return ErrNotStarted
	}
	p, err := tc.NewDockerProvider()
	if err != nil {
		return fmt.Errorf("unpause: %w", err)
	}
	defer func() { _ = p.Close() }()
	cli := p.Client()
	_, err = cli.ContainerUnpause(ctx, ctr.id, client.ContainerUnpauseOptions{})
	if err != nil {
		return fmt.Errorf("unpause: %w", err)
	}
	return nil
}

// Terminate terminates the started container. Returns [ErrNotStarted] when the
// container was never started.
func (ctr *CTR) Terminate(ctx context.Context) (err error) {
	if ctr.dc == nil {
		if ctr.terminated {
			return nil
		}
		return ErrNotStarted
	}
	tcc := ctr.dc
	ctr.cliMx.Lock()
	if ctr.cli != nil {
		_ = ctr.cli.Close()
		ctr.cli = nil
	}
	ctr.cliMx.Unlock()

	// Mark terminated only after the underlying terminate succeeds (or fails
	// with an ignorable error), so a real failure leaves the container bound
	// and a retry can try again instead of reporting success.
	if err = tcc.Terminate(ctx); handleErr(err) {
		return fmt.Errorf("terminate: %w", err)
	}
	ctr.dc = nil
	ctr.terminated = true

	if !ctr.removeImage {
		return nil
	}
	ref := ctr.reference
	if ref == "" {
		ref = ctr.req.Image
	}
	if ref == "" {
		return nil
	}
	return deleteImage(ctx, ref)
}

// deleteImage force-removes every image matching the reference.
func deleteImage(ctx context.Context, ref string) error {
	cli, err := tc.NewDockerClientWithOpts(ctx)
	if err != nil {
		return fmt.Errorf("remove image %s: %w", ref, err)
	}
	defer func() { _ = cli.Close() }()

	// The Docker reference filter matches only the familiar form of a name,
	// "ealen/echo-server:0.9.2", never "docker.io/ealen/echo-server:0.9.2".
	filter := ref
	if named, err := reference.ParseNormalizedNamed(ref); err == nil {
		filter = reference.FamiliarString(named)
	}
	lsOpts := client.ImageListOptions{
		Filters: make(client.Filters).Add("reference", filter),
	}
	list, err := cli.ImageList(ctx, lsOpts)
	if err != nil {
		return fmt.Errorf("remove image %s: %w", ref, err)
	}
	rmOpts := client.ImageRemoveOptions{Force: true, PruneChildren: true}
	var ers error
	for _, sum := range list.Items {
		imgID := strings.TrimPrefix(sum.ID, "sha256:")
		if _, err = cli.ImageRemove(ctx, imgID, rmOpts); err != nil {
			err = fmt.Errorf("remove image %s: %w", imgID, err)
			ers = errors.Join(ers, err)
		}
	}
	return ers
}

// handleErr returns true if the error is one we need to handle by returning
// it from a method, otherwise the error is ignored. A missing container and a
// removal already in progress are ignored.
func handleErr(err error) bool {
	switch {
	case err == nil, cerrdefs.IsNotFound(err):
		return false
	case cerrdefs.IsConflict(err):
		return !strings.Contains(err.Error(), "is already in progress")
	}
	return true
}

func (ctr *CTR) Describe() string {
	buf := strings.Builder{}
	buf.WriteString("\n")
	buf.WriteString(ctr.FormatInfoLine("---"))
	if !ctr.IsRunning() {
		buf.WriteString(ctr.FormatInfoLine("not started"))
		buf.WriteString(ctr.FormatInfoLine("---"))
		return buf.String()
	}
	buf.WriteString(ctr.FormatInfoLine("CTR Description:"))

	keys := slices.Sorted(maps.Keys(ctr.cfgHost))
	for _, key := range keys {
		msg := ctr.FormatInfoLine(
			"\t%s: %s -> %s",
			key,
			ctr.cfgHost[key],
			ctr.cfgGuest[key],
		)
		buf.WriteString(msg)
	}

	for _, fil := range DescribeFiles(ctr.req.Files) {
		buf.WriteString(ctr.FormatInfoLine("%s", fil))
	}

	buf.WriteString("---\n")
	return buf.String()
}

// FormatInfoLine returns the format string rendered with the arguments and
// prefixed with the container's short ID and name.
func (ctr *CTR) FormatInfoLine(format string, a ...any) string {
	id := "not started"
	if ctr.id != "" {
		id = ShortID(ctr.id)
	}
	line := fmt.Sprintf(format, a...)
	return fmt.Sprintf("\t>| (%s) [%s] > %s\n", id, ctr.name, line)
}

// startMeta holds provenance labels and default container env stamped at Start.
type startMeta struct {
	labels map[string]string
	env    map[string]string
}

// scmInfo is SCM provenance used when stamping a container.
type scmInfo struct {
	repo string
	rev  string
	hash string
}

// scmFromGit resolves origin, describe, and latest hash from the checkout.
// Credentials in the origin URL are dropped. Missing values fall back to
// [xdef] placeholders.
func scmFromGit(ctx context.Context) scmInfo {
	scm := scmInfo{
		repo: xdef.PhUnknown,
		rev:  xdef.PhTag,
		hash: xdef.PhHash,
	}
	if val, _ := gitaid.ProjectOrigin(ctx, ""); val != "" {
		scm.repo = stripUserinfo(val)
	}
	if val, _ := gitaid.Describe(ctx, ""); val != "" {
		scm.rev = val
	}
	if val, _ := gitaid.LatestHash(ctx, ""); val != "" {
		scm.hash = val
	}
	return scm
}

// buildMeta builds [startMeta] from env, container name, and SCM info.
// It is pure: no git or Docker calls.
func buildMeta(env []string, name string, scm scmInfo) startMeta {
	bldDate := xdef.BldDate(env)
	return startMeta{
		labels: map[string]string{
			xdef.LabImgSrc:     scm.repo,
			xdef.LabImgVer:     scm.rev,
			xdef.LabImgRev:     scm.hash,
			xdef.LabImgCreated: bldDate,
			LabTestCtrName:     name,
		},
		env: map[string]string{
			xdef.EnvBldDate: bldDate,
			xdef.EnvScmRepo: scm.repo,
			xdef.EnvScmRev:  scm.rev,
			xdef.EnvScmHash: scm.hash,
		},
	}
}

// prepareRequest returns a request ready for GenericContainer: provenance
// labels and env, optional build args, entrypoint override, and log consumer:
// the custom one when given, else a printing one when [EnvCTRLog] is "true".
func prepareRequest(
	req tc.GenericContainerRequest,
	meta startMeta,
	env []string,
	log tc.LogConsumer,
) tc.GenericContainerRequest {

	envMap := maps.Clone(meta.env)
	if envMap == nil {
		envMap = make(map[string]string)
	}

	if req.ShouldBuildImage() {
		if req.Repo == "" {
			req.Repo = RandName()
		}
		if req.Tag == "" {
			req.Tag = RandTag()
		}
		orig := req.BuildOptionsModifier
		labMap := meta.labels
		req.BuildOptionsModifier = func(opts *client.ImageBuildOptions) {
			if orig != nil {
				orig(opts)
			}
			opts.Labels = SetMissing(opts.Labels, labMap)
		}
		buildArgs := map[string]*string{
			xdef.EnvBldDate: new(meta.env[xdef.EnvBldDate]),
		}
		req.BuildArgs = SetMissing(req.BuildArgs, buildArgs)
		if ring.EnvGet(env, EnvCTRBuildLog) == "true" {
			req.FromDockerfile.BuildLogWriter = os.Stdout
		}
	} else {
		req.Labels = SetMissing(req.Labels, meta.labels)
	}

	// Docker appends Cmd to the entrypoint as arguments; drop it so the
	// override runs alone.
	if cmd := ring.EnvGet(env, EnvCTREntrypoint); cmd != "" {
		req.WaitingFor = nil
		req.Entrypoint = strings.Fields(cmd)
		req.Cmd = nil
	}

	req.Env = SetMissing(req.Env, envMap)

	if req.LogConsumerCfg == nil {
		lc := log
		if lc == nil && ring.EnvGet(env, EnvCTRLog) == "true" {
			// Nothing can read the lines of this logger; only print them.
			lgr := NewLogger(true)
			lgr.drop = true
			lc = lgr
		}
		if lc != nil {
			req.LogConsumerCfg = &tc.LogConsumerConfig{
				Consumers: []tc.LogConsumer{lc},
			}
		}
	}

	// Loggers prefix printed lines with the container ID, known only once
	// the container is created.
	var lgrs []*Logger
	if req.LogConsumerCfg != nil {
		for _, lc := range req.LogConsumerCfg.Consumers {
			if lgr, ok := lc.(*Logger); ok {
				lgrs = append(lgrs, lgr)
			}
		}
	}
	if len(lgrs) > 0 {
		hook := func(_ context.Context, tcc tc.Container) error {
			for _, lgr := range lgrs {
				lgr.SetCID(tcc.GetContainerID())
			}
			return nil
		}
		hooks := tc.ContainerLifecycleHooks{
			PostCreates: []tc.ContainerHook{hook},
		}
		req.LifecycleHooks = append(slices.Clone(req.LifecycleHooks), hooks)
	}

	return req
}

// Start prepares a copy of the configured request, so it may be called again
// after [CTR.Terminate]; [CTR.Request] keeps returning the configured request.
func (ctr *CTR) Start(ctx context.Context, env []string) error {
	if ctr.dc != nil {
		return ErrRunning
	}
	// A previous build read the context archive to its end.
	if arc := ctr.req.ContextArchive; arc != nil {
		if _, err := arc.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("rewind context archive: %w", err)
		}
	}

	meta := buildMeta(env, ctr.name, scmFromGit(ctx))
	ctr.runReq = prepareRequest(cloneReq(ctr.req), meta, env, ctr.log)
	return ctr.startAndBind(ctx)
}

// startAndBind runs GenericContainer and binds the result onto CTR.
func (ctr *CTR) startAndBind(ctx context.Context) error {
	started, err := tc.GenericContainer(ctx, ctr.runReq)
	if err != nil {
		// A container that was created but failed to start is still returned
		// and must be terminated, or it keeps running.
		err = fmt.Errorf("start: %w", err)
		return errors.Join(err, tc.TerminateContainer(started))
	}
	dc, ok := started.(*tc.DockerContainer)
	if !ok {
		_ = started.Terminate(ctx)
		return errors.New("not *tc.DockerContainer instance")
	}
	ctr.dc = dc
	if err = ctr.bindStarted(ctx); err != nil {
		// Without its connection config the container is unusable; do not
		// leave it running behind an error.
		ctr.dc = nil
		return errors.Join(err, tc.TerminateContainer(dc))
	}
	ctr.RegisterCleanup(ctr.Terminate)
	return nil
}

// bindStarted records runtime identity after the container has been created,
// and the host/guest connection config when it has also been started.
func (ctr *CTR) bindStarted(ctx context.Context) error {
	ctr.id = ctr.dc.GetContainerID()
	ctr.reference = ctr.dc.Image
	if !ctr.runReq.Started {
		return nil
	}
	ctr.cfgHost = make(map[string]string)
	ctr.cfgGuest = make(map[string]string)

	for i, ep := range ctr.runReq.ExposedPorts {
		if _, after, ok := strings.Cut(ep, ":"); ok {
			ep = after
		}
		port, err := ctr.dc.MappedPort(ctx, ep)
		if err != nil {
			return fmt.Errorf("mapped port %s: %w", ep, err)
		}
		ctr.cfgHost["PORT_"+strconv.Itoa(i)] = port.String()
		ctr.cfgGuest["PORT_"+strconv.Itoa(i)] = ep
	}

	var err error
	if ctr.cfgHost["HOST"], err = ctr.dc.Host(ctx); err != nil {
		return fmt.Errorf("host: %w", err)
	}
	if ctr.cfgGuest["HOST"], err = ctr.dc.ContainerIP(ctx); err != nil {
		return fmt.Errorf("container ip: %w", err)
	}

	return nil
}
