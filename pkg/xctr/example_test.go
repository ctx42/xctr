// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xctr_test

import (
	"context"
	"fmt"
	"os"

	"github.com/ctx42/xctr/pkg/xctr"
)

func ExampleRef() {
	fmt.Println(xctr.Ref("example.com", "app", "v1"))
	// Output: example.com/app:v1
}

func ExampleParseRef() {
	ref := "registry.example.com:5000/team/app:v1.2.3"
	repo, name, tag := xctr.ParseRef(ref)
	fmt.Printf("%s | %s | %s\n", repo, name, tag)
	// Output: registry.example.com:5000 | team/app | v1.2.3
}

func ExampleShortID() {
	fmt.Println(xctr.ShortID("0123456789abcdef"))
	// Output: 01234567
}

func ExampleArchive() {
	rdr := xctr.MustArchive(
		xctr.WithArchFile(xctr.NewArchFile("hello.txt", 0o0644, []byte("hi"))),
	)

	arch, err := xctr.Unarchive(rdr)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s: %s\n", arch.Files[0].Name, arch.Files[0].Content)
	// Output: hello.txt: hi
}

func ExampleNewCTR() {
	ctx := context.Background()
	ctr := xctr.NewCTR("busybox", xctr.ImageReq("busybox:1.38-uclibc"))
	if err := ctr.Start(ctx, os.Environ()); err != nil {
		panic(err)
	}
	defer func() { _ = ctr.Cleanup(ctx) }()

	res := ctr.Exec(ctx, "echo", "hello")
	if err := res.Err(); err != nil {
		panic(err)
	}
	fmt.Print(res.SOut)
}

func ExampleWhitelist_Check() {
	wl := xctr.NewWhitelist()
	if err := wl.Add("ls .*", "cat /etc/hostname"); err != nil {
		panic(err)
	}

	fmt.Println(wl.Check("ls", "-la"))
	fmt.Println(wl.Check("cat", "/etc/hostname", "/etc/shadow"))
	// Output:
	// <nil>
	// not allowed: cat /etc/hostname /etc/shadow
}

func ExampleOnce() {
	onc := xctr.NewOnce()
	ctr := xctr.NewCTR("busybox", xctr.ImageReq("busybox:1.38-uclibc"))

	fmt.Println(onc.Add(ctr))
	fmt.Println(onc.Add(ctr))
	fmt.Println(onc.Names())
	// Output:
	// true
	// false
	// [busybox]
}
