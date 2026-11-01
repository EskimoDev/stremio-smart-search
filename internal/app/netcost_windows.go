//go:build windows

package app

import (
	"runtime"
	"syscall"
	"unsafe"
)

// networkCost asks Windows' Network List Manager (INetworkCostManager) for the
// machine-wide connection cost. This is the same flag Settings shows as
// "Metered connection". Any failure reports unknown.

// ole32 procs and the guid type are shared with picker_windows.go.
var (
	clsidNetworkListManager = guid{0xDCB00C01, 0x570F, 0x4A9B, [8]byte{0x8D, 0x69, 0x19, 0x9F, 0xDB, 0xA5, 0x72, 0x3B}}
	iidINetworkCostManager  = guid{0xDCB00008, 0x570F, 0x4A9B, [8]byte{0x8D, 0x69, 0x19, 0x9F, 0xDB, 0xA5, 0x72, 0x3B}}
)

// INetworkCostManager vtable: IUnknown (0-2), GetCost (3), ...
type costManager struct{ vtbl *[6]uintptr }

const (
	clsctxAll            = 0x17
	nlmCostUnrestricted  = 0x1
	nlmCostFixed         = 0x2
	nlmCostVariable      = 0x4
	nlmCostOverDataLimit = 0x10000
	nlmCostRoaming       = 0x40000
	coinitMultithreaded  = 0x0
)

func networkCost() string {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if hr, _, _ := procCoInitializeEx.Call(0, coinitMultithreaded); int32(hr) >= 0 {
		defer procCoUninitialize.Call()
	}
	var manager *costManager
	hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidNetworkListManager)), 0, clsctxAll,
		uintptr(unsafe.Pointer(&iidINetworkCostManager)), uintptr(unsafe.Pointer(&manager)))
	if int32(hr) < 0 || manager == nil {
		return "unknown"
	}
	defer syscall.SyscallN(manager.vtbl[2], uintptr(unsafe.Pointer(manager)))
	var cost uint32
	hr, _, _ = syscall.SyscallN(manager.vtbl[3], uintptr(unsafe.Pointer(manager)), uintptr(unsafe.Pointer(&cost)), 0)
	if int32(hr) < 0 {
		return "unknown"
	}
	return costLabel(cost)
}

func costLabel(cost uint32) string {
	switch {
	case cost&(nlmCostFixed|nlmCostVariable|nlmCostOverDataLimit|nlmCostRoaming) != 0:
		return "metered"
	case cost&nlmCostUnrestricted != 0:
		return "unmetered"
	}
	return "unknown"
}
