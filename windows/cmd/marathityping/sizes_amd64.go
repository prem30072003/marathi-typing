//go:build windows && amd64

package main

import "unsafe"

// Compile-time checks that the Go structs match the Win32 x64 layouts.
var (
	_ [40]byte  = [unsafe.Sizeof(kbInput{})]byte{}
	_ [40]byte  = [unsafe.Sizeof(msInput{})]byte{}
	_ [24]byte  = [unsafe.Sizeof(KBDLLHOOKSTRUCT{})]byte{}
	_ [32]byte  = [unsafe.Sizeof(MSLLHOOKSTRUCT{})]byte{}
	_ [976]byte = [unsafe.Sizeof(NOTIFYICONDATAW{})]byte{}
	_ [72]byte  = [unsafe.Sizeof(GUITHREADINFO{})]byte{}
	_ [48]byte  = [unsafe.Sizeof(MSG{})]byte{}
	_ [80]byte  = [unsafe.Sizeof(WNDCLASSEXW{})]byte{}
	_ [72]byte  = [unsafe.Sizeof(PAINTSTRUCT{})]byte{}
	_ [40]byte  = [unsafe.Sizeof(MONITORINFO{})]byte{}
	_ [32]byte  = [unsafe.Sizeof(ICONINFO{})]byte{}
)
