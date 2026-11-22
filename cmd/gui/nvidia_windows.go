//go:build windows

package main

/*
// Instruct hybrid GPU drivers (NVIDIA Optimus / AMD Enduro) not to wake the
// discrete GPU for this lightweight diagnostic UI when an integrated GPU exists.
__declspec(dllexport) unsigned long NvOptimusEnablement = 0x00000000;
__declspec(dllexport) int AmdPowerXpressRequestHighPerformance = 0;
*/
import "C"

import (
	"syscall"
	"unsafe"
)

const (
	processExtensionPointDisablePolicy = 6
	processSignaturePolicy             = 8
)

type extensionPointDisablePolicy struct {
	Flags uint32
}

type binarySignaturePolicy struct {
	Flags uint32
}

func init() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setMitigation := kernel32.NewProc("SetProcessMitigationPolicy")
	if setMitigation.Find() != nil {
		return
	}

	// 1. Disable legacy global DLL injection / Win32 hook extension points (AppInit_DLLs, global hooks).
	extPolicy := extensionPointDisablePolicy{Flags: 1} // DisableExtensionPoints = 1
	_, _, _ = setMitigation.Call(
		uintptr(processExtensionPointDisablePolicy),
		uintptr(unsafe.Pointer(&extPolicy)),
		unsafe.Sizeof(extPolicy),
	)

	// 2. Enforce Microsoft/WHQL-signed binaries only so third-party game overlay hooks
	//    (such as NVIDIA ShadowPlay / GeForce Experience nvspcap64.dll) cannot inject
	//    into the OpenGL rendering context while Microsoft WHQL GPU drivers still load.
	sigPolicy := binarySignaturePolicy{Flags: 1} // MicrosoftSignedOnly = 1
	_, _, _ = setMitigation.Call(
		uintptr(processSignaturePolicy),
		uintptr(unsafe.Pointer(&sigPolicy)),
		unsafe.Sizeof(sigPolicy),
	)
}
