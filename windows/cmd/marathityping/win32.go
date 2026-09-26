//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	pSetWindowsHookExW          = user32.NewProc("SetWindowsHookExW")
	pCallNextHookEx             = user32.NewProc("CallNextHookEx")
	pUnhookWindowsHookEx        = user32.NewProc("UnhookWindowsHookEx")
	pGetMessageW                = user32.NewProc("GetMessageW")
	pTranslateMessage           = user32.NewProc("TranslateMessage")
	pDispatchMessageW           = user32.NewProc("DispatchMessageW")
	pPostQuitMessage            = user32.NewProc("PostQuitMessage")
	pRegisterClassExW           = user32.NewProc("RegisterClassExW")
	pCreateWindowExW            = user32.NewProc("CreateWindowExW")
	pDefWindowProcW             = user32.NewProc("DefWindowProcW")
	pShowWindow                 = user32.NewProc("ShowWindow")
	pSetWindowPos               = user32.NewProc("SetWindowPos")
	pInvalidateRect             = user32.NewProc("InvalidateRect")
	pBeginPaint                 = user32.NewProc("BeginPaint")
	pEndPaint                   = user32.NewProc("EndPaint")
	pGetDC                      = user32.NewProc("GetDC")
	pReleaseDC                  = user32.NewProc("ReleaseDC")
	pFillRect                   = user32.NewProc("FillRect")
	pDrawTextW                  = user32.NewProc("DrawTextW")
	pSendInput                  = user32.NewProc("SendInput")
	pGetAsyncKeyState           = user32.NewProc("GetAsyncKeyState")
	pGetKeyState                = user32.NewProc("GetKeyState")
	pGetForegroundWindow        = user32.NewProc("GetForegroundWindow")
	pGetWindowThreadProcId      = user32.NewProc("GetWindowThreadProcessId")
	pGetGUIThreadInfo           = user32.NewProc("GetGUIThreadInfo")
	pClientToScreen             = user32.NewProc("ClientToScreen")
	pGetWindowRect              = user32.NewProc("GetWindowRect")
	pMonitorFromPoint           = user32.NewProc("MonitorFromPoint")
	pGetMonitorInfoW            = user32.NewProc("GetMonitorInfoW")
	pRegisterHotKey             = user32.NewProc("RegisterHotKey")
	pCreatePopupMenu            = user32.NewProc("CreatePopupMenu")
	pAppendMenuW                = user32.NewProc("AppendMenuW")
	pTrackPopupMenu             = user32.NewProc("TrackPopupMenu")
	pDestroyMenu                = user32.NewProc("DestroyMenu")
	pGetCursorPos               = user32.NewProc("GetCursorPos")
	pSetForegroundWindow        = user32.NewProc("SetForegroundWindow")
	pMessageBoxW                = user32.NewProc("MessageBoxW")
	pCreateIconIndirect         = user32.NewProc("CreateIconIndirect")
	pDestroyIcon                = user32.NewProc("DestroyIcon")
	pSetTimer                   = user32.NewProc("SetTimer")
	pKillTimer                  = user32.NewProc("KillTimer")
	pGetWindowLongPtrW          = user32.NewProc("GetWindowLongPtrW")
	pGetClassNameW              = user32.NewProc("GetClassNameW")
	pSetProcessDPIAware         = user32.NewProc("SetProcessDPIAware")
	pGetDpiForSystem            = user32.NewProc("GetDpiForSystem")
	pPostMessageW               = user32.NewProc("PostMessageW")
	pLoadCursorW                = user32.NewProc("LoadCursorW")
	pRegisterWindowMessageW     = user32.NewProc("RegisterWindowMessageW")
	pLoadImageW                 = user32.NewProc("LoadImageW")
	pGetSystemMetrics           = user32.NewProc("GetSystemMetrics")
	pSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	pSetWindowRgn               = user32.NewProc("SetWindowRgn")
	pSetCapture                 = user32.NewProc("SetCapture")
	pReleaseCapture             = user32.NewProc("ReleaseCapture")
	pIsWindowVisible            = user32.NewProc("IsWindowVisible")
	pCreateRoundRectRgn         = gdi32.NewProc("CreateRoundRectRgn")
	pShellExecuteW              = shell32.NewProc("ShellExecuteW")
	pCreateFontW                = gdi32.NewProc("CreateFontW")
	pSelectObject               = gdi32.NewProc("SelectObject")
	pDeleteObject               = gdi32.NewProc("DeleteObject")
	pSetTextColor               = gdi32.NewProc("SetTextColor")
	pSetBkMode                  = gdi32.NewProc("SetBkMode")
	pCreateSolidBrush           = gdi32.NewProc("CreateSolidBrush")
	pCreateCompatibleDC         = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap     = gdi32.NewProc("CreateCompatibleBitmap")
	pCreateBitmap               = gdi32.NewProc("CreateBitmap")
	pDeleteDC                   = gdi32.NewProc("DeleteDC")
	pGetTextExtentPoint32W      = gdi32.NewProc("GetTextExtentPoint32W")
	pGetModuleHandleW           = kernel32.NewProc("GetModuleHandleW")
	pCreateMutexW               = kernel32.NewProc("CreateMutexW")
	pGetCurrentProcessId        = kernel32.NewProc("GetCurrentProcessId")
	pShellNotifyIconW           = shell32.NewProc("Shell_NotifyIconW")
	pRegOpenKeyExW              = advapi32.NewProc("RegOpenKeyExW")
	pRegSetValueExW             = advapi32.NewProc("RegSetValueExW")
	pRegDeleteValueW            = advapi32.NewProc("RegDeleteValueW")
	pRegQueryValueExW           = advapi32.NewProc("RegQueryValueExW")
	pRegCloseKey                = advapi32.NewProc("RegCloseKey")
)

const (
	WH_KEYBOARD_LL = 13
	WH_MOUSE_LL    = 14

	WM_DESTROY       = 0x0002
	WM_PAINT         = 0x000F
	WM_CLOSE         = 0x0010
	WM_KEYDOWN       = 0x0100
	WM_KEYUP         = 0x0101
	WM_SYSKEYDOWN    = 0x0104
	WM_SYSKEYUP      = 0x0105
	WM_COMMAND       = 0x0111
	WM_TIMER         = 0x0113
	WM_HOTKEY        = 0x0312
	WM_LBUTTONDOWN   = 0x0201
	WM_LBUTTONUP     = 0x0202
	WM_RBUTTONDOWN   = 0x0204
	WM_RBUTTONUP     = 0x0205
	WM_MBUTTONDOWN   = 0x0207
	WM_MOUSEACTIVATE = 0x0021
	MA_NOACTIVATE    = 3
	WM_MOUSEMOVE     = 0x0200
	WS_EX_LAYERED    = 0x00080000
	LWA_ALPHA        = 0x2
	SWP_NOSIZE       = 0x0001
	SM_CXSMICON      = 49
	SM_CYSMICON      = 50
	IMAGE_ICON       = 1
	LR_LOADFROMFILE  = 0x0010
	VK_NUMPAD0       = 0x60
	VK_OEM_PERIOD    = 0xBE
	VK_OEM_2         = 0xBF
	WM_APP           = 0x8000
	WM_TRAY          = WM_APP + 1

	WS_POPUP          = 0x80000000
	WS_BORDER         = 0x00800000
	WS_EX_TOPMOST     = 0x00000008
	WS_EX_TOOLWINDOW  = 0x00000080
	WS_EX_NOACTIVATE  = 0x08000000
	SW_HIDE           = 0
	SW_SHOWNOACTIVATE = 4
	SWP_NOACTIVATE    = 0x0010
	SWP_SHOWWINDOW    = 0x0040
	HWND_TOPMOST      = ^uintptr(0) // -1
	CS_DROPSHADOW     = 0x00020000

	LLKHF_INJECTED = 0x10
	LLMHF_INJECTED = 0x01

	INPUT_MOUSE            = 0
	INPUT_KEYBOARD         = 1
	KEYEVENTF_EXTENDEDKEY  = 0x0001
	KEYEVENTF_KEYUP        = 0x0002
	KEYEVENTF_UNICODE      = 0x0004
	KEYEVENTF_SCANCODE     = 0x0008
	MOUSEEVENTF_LEFTDOWN   = 0x0002
	MOUSEEVENTF_RIGHTDOWN  = 0x0008
	MOUSEEVENTF_MIDDLEDOWN = 0x0020

	VK_BACK     = 0x08
	VK_TAB      = 0x09
	VK_RETURN   = 0x0D
	VK_SHIFT    = 0x10
	VK_CONTROL  = 0x11
	VK_MENU     = 0x12
	VK_CAPITAL  = 0x14
	VK_ESCAPE   = 0x1B
	VK_SPACE    = 0x20
	VK_UP       = 0x26
	VK_DOWN     = 0x28
	VK_LWIN     = 0x5B
	VK_RWIN     = 0x5C
	VK_NUMPAD1  = 0x61
	VK_NUMPAD9  = 0x69
	VK_LSHIFT   = 0xA0
	VK_RSHIFT   = 0xA1
	VK_LCONTROL = 0xA2
	VK_RCONTROL = 0xA3
	VK_LMENU    = 0xA4
	VK_RMENU    = 0xA5

	MOD_ALT      = 0x0001
	MOD_NOREPEAT = 0x4000

	NIM_ADD         = 0
	NIM_MODIFY      = 1
	NIM_DELETE      = 2
	NIF_MESSAGE     = 0x01
	NIF_ICON        = 0x02
	NIF_TIP         = 0x04
	NIF_INFO        = 0x10
	NIIF_INFO       = 0x01
	MF_STRING       = 0x0000
	MF_SEPARATOR    = 0x0800
	MF_CHECKED      = 0x0008
	MF_GRAYED       = 0x0001
	TPM_RETURNCMD   = 0x0100
	TPM_RIGHTBUTTON = 0x0002

	DT_LEFT       = 0x0000
	DT_CENTER     = 0x0001
	DT_VCENTER    = 0x0004
	DT_SINGLELINE = 0x0020
	DT_NOPREFIX   = 0x0800
	TRANSPARENT   = 1

	HKEY_CURRENT_USER = 0x80000001
	KEY_READ          = 0x20019
	KEY_WRITE         = 0x20006
	REG_SZ            = 1

	GWL_STYLE   = -16
	ES_PASSWORD = 0x0020

	MB_OK              = 0x0000
	MB_ICONINFORMATION = 0x0040
	MB_ICONWARNING     = 0x0030
)

type POINT struct{ X, Y int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
	_       uint32
}

type KBDLLHOOKSTRUCT struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type MSLLHOOKSTRUCT struct {
	Pt          POINT
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type keybdInput struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type kbInput struct {
	Type uint32
	_    uint32
	Ki   keybdInput
	_    [8]byte
}

type mouseInputS struct {
	Dx, Dy      int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type msInput struct {
	Type uint32
	_    uint32
	Mi   mouseInputS
}

type GUITHREADINFO struct {
	CbSize        uint32
	Flags         uint32
	HwndActive    uintptr
	HwndFocus     uintptr
	HwndCapture   uintptr
	HwndMenuOwner uintptr
	HwndMoveSize  uintptr
	HwndCaret     uintptr
	RcCaret       RECT
}

type MONITORINFO struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
}

type NOTIFYICONDATAW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

type ICONINFO struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

type SIZE struct{ CX, CY int32 }

func u16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

func copyU16(dst []uint16, s string) {
	u, _ := syscall.UTF16FromString(s)
	if len(u) > len(dst) {
		u = u[:len(dst)]
		u[len(u)-1] = 0
	}
	copy(dst, u)
}

func rgb(r, g, b byte) uintptr { return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16 }

func messageBox(text, title string, flags uintptr) {
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(u16(text))), uintptr(unsafe.Pointer(u16(title))), flags)
}

func keyDown(vk int) bool {
	r, _, _ := pGetAsyncKeyState.Call(uintptr(vk))
	return int16(r) < 0
}
