// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"github.com/distribution/reference"
	"github.com/moby/moby/api/types/container"
	tc "github.com/testcontainers/testcontainers-go"
)

// ImageReq returns a container request for the given image reference.
func ImageReq(ref string) tc.GenericContainerRequest {
	return tc.GenericContainerRequest{
		Started: true,
		ContainerRequest: tc.ContainerRequest{
			Image: ref,
			HostConfigModifier: func(hc *container.HostConfig) {
				hc.AutoRemove = true
			},
			Entrypoint: []string{"tail", "-f", "/dev/null"},
		},
	}
}

// ToBuildArgs takes a map of string to string and returns a map of string
// to a pointer to string. Useful to create the build arguments map.
func ToBuildArgs(ba map[string]string) map[string]*string {
	args := make(map[string]*string, len(ba))
	for k, v := range ba {
		args[k] = new(v)
	}
	return args
}

// Ref returns an image reference based on the repo name, image name, and image
// tag. A tag in digest form, "sha256:<hex>", is joined with "@".
func Ref(repo, name, tag string) string {
	ref := strings.TrimRight(repo, "/")
	if name == "" {
		return ref
	}
	if ref != "" {
		ref += "/"
	}
	ref += name
	if tag == "" {
		return ref
	}
	// A tag never contains a colon; a digest always does.
	if strings.Contains(tag, ":") {
		return ref + "@" + tag
	}
	return ref + ":" + tag
}

// ParseRef parses a reference and returns the repository, name, and tag. On
// error returns empty strings.
func ParseRef(ref string) (string, string, string) {
	namedRef, err := reference.ParseAnyReference(ref)
	if err != nil {
		return "", "", ""
	}
	named, ok := namedRef.(reference.Named)
	if !ok {
		return "", "", ""
	}

	var tag string
	if digested, ok := namedRef.(reference.Digested); ok { //nolint:gocritic
		tag = digested.Digest().String()
	} else if tagged, ok := namedRef.(reference.Tagged); ok {
		tag = tagged.Tag()
	} else {
		tag = "latest"
	}

	repo := reference.Domain(named)
	path := reference.Path(named)
	if repo == "docker.io" && strings.HasPrefix(path, "library/") {
		path, _ = strings.CutPrefix(path, "library/")
	}

	return repo, path, tag
}

// RandName returns a random Docker image name. All returned strings are
// 12-character random strings prefixed with "c42-tst-img-".
func RandName() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return "c42-tst-img-" + hex.EncodeToString(buf)
}

// RandTag returns a random Docker image tag. All returned strings are
// 12-character random strings prefixed with "c42-tst-tag-".
func RandTag() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return "c42-tst-tag-" + hex.EncodeToString(buf)
}

// RandRef returns random Docker reference (image-name:image-tag). Both are
// prefixed with "c42-tst-img-" and "c42-tst-tag-" respectively.
func RandRef() string {
	return RandName() + ":" + RandTag()
}

// RandNet returns random Docker network name. All returned strings are 12
// character random strings prefixed with "c42-tst-net-".
func RandNet() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return "c42-tst-net-" + hex.EncodeToString(buf)
}

// SetMissing copies into dst the keys that are in src but not already in dst,
// and returns dst. When dst is nil a new map is allocated, so the result is
// never nil.
func SetMissing[K comparable, V any](dst, src map[K]V) map[K]V {
	if dst == nil {
		dst = make(map[K]V, len(src))
	}
	for key, val := range src {
		if _, ok := dst[key]; !ok {
			dst[key] = val
		}
	}
	return dst
}

// ShortID shortens the ID to 8 characters. It returns the ID if it's shorter.
func ShortID(id string) string {
	if len(id) < 8 {
		return id
	}
	return id[:8]
}

// DescribeFiles returns lines describing the files slice.
func DescribeFiles(fls []tc.ContainerFile) []string {
	lns := make([]string, 0, len(fls))
	if len(fls) > 0 {
		lns = append(lns, "Files added:")
	}
	for _, fil := range fls {
		hostPth := fil.HostFilePath
		if hostPth == "" {
			hostPth = "<reader>"
		}
		ctrPth := fil.ContainerFilePath
		lns = append(lns, fmt.Sprintf("\t%s -> %s", hostPth, ctrPth))
	}
	return lns
}

// stripUserinfo returns the URL without its user name and password, which may
// carry an access token. A value that does not parse as a URL, such as the
// scp-like "git@example.com:org/repo.git", is returned unchanged.
func stripUserinfo(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
}
