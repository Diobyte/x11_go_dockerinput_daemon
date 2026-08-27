//go:build cgo

package main

/*
#cgo LDFLAGS: -lX11 -lXtst
#define _GNU_SOURCE
#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <X11/Xutil.h>
#include <X11/extensions/XTest.h>
#include <X11/keysym.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/socket.h>

Display *dpy = NULL;

int x_io_error_handler(Display *d) {
	fprintf(stderr, "xtest-server: fatal X I/O error\n");
	_exit(1);
	return 0;
}

int x_error_handler(Display *d, XErrorEvent *e) {
	char buf[256];
	XGetErrorText(d, e->error_code, buf, sizeof(buf));
	fprintf(stderr, "xtest-server: X error: %s (request %d.%d)\n",
		buf, e->request_code, e->minor_code);
	return 0;
}

int screen_w = 1280, screen_h = 720;

int display_width() { return screen_w; }
int display_height() { return screen_h; }

void clear_modifier_lock(Display *d, KeyCode kc) {
	if (!kc) return;
	// Toggle twice to guarantee "off" regardless of initial state.
	XTestFakeKeyEvent(d, kc, True, 0);
	usleep(10000);
	XTestFakeKeyEvent(d, kc, False, 0);
	XTestFakeKeyEvent(d, kc, True, 0);
	usleep(10000);
	XTestFakeKeyEvent(d, kc, False, 0);
}

int init_display() {
	if (!XInitThreads()) {
		fprintf(stderr, "xtest-server: XInitThreads failed\n");
		return -1;
	}
	for (int i = 0; i < 15 && !dpy; i++) {
		dpy = XOpenDisplay(NULL);
		if (!dpy) usleep(200000);
	}
	if (!dpy) return -1;
	// GNOME Xwayland is started with -enable-ei-portal. XTEST and
	// XWarpPointer on that DISPLAY become a seat-global RemoteDesktop /
	// emulated-input portal request. Dest is container Xorg only.
	{
		int opcode = 0, event = 0, err = 0;
		if (XQueryExtension(dpy, "XWAYLAND", &opcode, &event, &err)) {
			fprintf(stderr, "xtest-server: DISPLAY is Xwayland; refusing host-seat EI/RemoteDesktop portal\n");
			XCloseDisplay(dpy);
			dpy = NULL;
			return -2;
		}
	}
	XSetErrorHandler(x_error_handler);
	XSetIOErrorHandler(x_io_error_handler);
	screen_w = DisplayWidth(dpy, DefaultScreen(dpy));
	screen_h = DisplayHeight(dpy, DefaultScreen(dpy));
	fprintf(stderr, "xtest-server: display %dx%d\n", screen_w, screen_h);
	XAutoRepeatOff(dpy);
	// Clear modifier locks (CapsLock, NumLock, ScrollLock).  A stray CapsLock
	// capitalises hotkey letters ("F1" → shift+F1) and D2R silently ignores
	// the wrong keysym.  x11vnc's clear_locks() does the same on startup.
	// Toggle each lock twice to guarantee "off" regardless of initial state.
	clear_modifier_lock(dpy, XKeysymToKeycode(dpy, XK_Caps_Lock));
	clear_modifier_lock(dpy, XKeysymToKeycode(dpy, XK_Num_Lock));
	clear_modifier_lock(dpy, XKeysymToKeycode(dpy, XK_Scroll_Lock));
	XSync(dpy, False);
	return 0;
}

int release_all() {
	if (!dpy) return -1;
	char keys[32];
	XQueryKeymap(dpy, keys);
	for (int kc = 0; kc < 256; kc++) {
		if (keys[kc >> 3] & (1 << (kc & 7))) {
			XTestFakeKeyEvent(dpy, kc, False, 0);
		}
	}
	XTestFakeButtonEvent(dpy, 1, False, 0);
	XTestFakeButtonEvent(dpy, 2, False, 0);
	XTestFakeButtonEvent(dpy, 3, False, 0);
	XSync(dpy, False);
	return 0;
}

int send_key(unsigned int keycode, int is_press) {
	if (!dpy) return -1;
	XTestFakeKeyEvent(dpy, keycode, is_press, 0);
	XSync(dpy, False);
	return 0;
}

int move_mouse(int x, int y) {
	if (!dpy) return -1;
	if (x < 0) x = 0;
	if (y < 0) y = 0;
	if (x >= screen_w) x = screen_w - 1;
	if (y >= screen_h) y = screen_h - 1;
	XWarpPointer(dpy, None, DefaultRootWindow(dpy), 0, 0, 0, 0, x, y);
	XSync(dpy, False);
	return 0;
}

int send_button(unsigned int button, int is_press) {
	if (!dpy) return -1;
	XTestFakeButtonEvent(dpy, button, is_press, 0);
	XSync(dpy, False);
	return 0;
}

unsigned int keysym_to_keycode(const char *keysym_str) {
	if (!dpy) return 0;
	KeySym ks = XStringToKeysym(keysym_str);
	if (ks == NoSymbol) {
		unsigned long hex;
		if (sscanf(keysym_str, "0x%lx", &hex) == 1) {
			ks = (KeySym)hex;
		}
	}
	if (ks == NoSymbol) return 0;
	return XKeysymToKeycode(dpy, ks);
}

static Atom a_net_supported, a_net_client_list, a_net_active, a_net_wm_state,
	a_net_wm_state_fs, a_net_wm_pid, a_net_wm_name, a_utf8;
static int dest_atoms_ready = 0;

int intern_dest_atoms() {
	if (!dpy) return -1;
	a_net_supported = XInternAtom(dpy, "_NET_SUPPORTED", False);
	a_net_client_list = XInternAtom(dpy, "_NET_CLIENT_LIST", False);
	a_net_active = XInternAtom(dpy, "_NET_ACTIVE_WINDOW", False);
	a_net_wm_state = XInternAtom(dpy, "_NET_WM_STATE", False);
	a_net_wm_state_fs = XInternAtom(dpy, "_NET_WM_STATE_FULLSCREEN", False);
	a_net_wm_pid = XInternAtom(dpy, "_NET_WM_PID", False);
	a_net_wm_name = XInternAtom(dpy, "_NET_WM_NAME", False);
	a_utf8 = XInternAtom(dpy, "UTF8_STRING", False);
	dest_atoms_ready = 1;
	return 0;
}

static int get_atom_list(Window w, Atom prop, Atom want_type, Atom *out, int maxn) {
	Atom type;
	int fmt;
	unsigned long n = 0, bytes = 0;
	unsigned char *data = NULL;
	if (!dpy) return -1;
	if (XGetWindowProperty(dpy, w, prop, 0, (long)maxn, False, want_type,
		&type, &fmt, &n, &bytes, &data) != Success || !data) {
		if (data) XFree(data);
		return -1;
	}
	if (type != want_type || fmt != 32) {
		XFree(data);
		return -1;
	}
	if (n > (unsigned long)maxn) n = (unsigned long)maxn;
	int i;
	for (i = 0; i < (int)n; i++) {
		out[i] = ((Atom *)data)[i];
	}
	XFree(data);
	return (int)n;
}

int net_supported_has(const char *name) {
	if (!dpy || !dest_atoms_ready || !name) return -1;
	Atom want = XInternAtom(dpy, name, False);
	Atom atoms[256];
	Window root = DefaultRootWindow(dpy);
	int n = get_atom_list(root, a_net_supported, XA_ATOM, atoms, 256);
	if (n < 0) return 0;
	int i;
	for (i = 0; i < n; i++) {
		if (atoms[i] == want) return 1;
	}
	return 0;
}

int refresh_client_list(unsigned int *xids, int maxn) {
	if (!dpy || !dest_atoms_ready || !xids || maxn <= 0) return -1;
	Atom type;
	int fmt;
	unsigned long n = 0, bytes = 0;
	unsigned char *data = NULL;
	Window root = DefaultRootWindow(dpy);
	if (XGetWindowProperty(dpy, root, a_net_client_list, 0, (long)maxn, False, XA_WINDOW,
		&type, &fmt, &n, &bytes, &data) != Success || !data) {
		if (data) XFree(data);
		return -1;
	}
	if (type != XA_WINDOW || fmt != 32) {
		XFree(data);
		return -1;
	}
	if (n > (unsigned long)maxn) n = (unsigned long)maxn;
	int i;
	for (i = 0; i < (int)n; i++) {
		xids[i] = (unsigned int)((Window *)data)[i];
	}
	XFree(data);
	return (int)n;
}

int window_geometry(unsigned int xid, int *x, int *y, int *w, int *h) {
	if (!dpy || !xid) return -1;
	Window root_ret, child;
	int rx, ry;
	unsigned int wr, hr, bw, depth;
	if (!XGetGeometry(dpy, (Window)xid, &root_ret, x, y, &wr, &hr, &bw, &depth)) {
		return -1;
	}
	XTranslateCoordinates(dpy, (Window)xid, DefaultRootWindow(dpy), 0, 0, &rx, &ry, &child);
	*x = rx;
	*y = ry;
	*w = (int)wr;
	*h = (int)hr;
	return 0;
}

unsigned int window_pid(unsigned int xid) {
	if (!dpy || !dest_atoms_ready || !xid) return 0;
	Atom type;
	int fmt;
	unsigned long n = 0, bytes = 0;
	unsigned char *data = NULL;
	if (XGetWindowProperty(dpy, (Window)xid, a_net_wm_pid, 0, 1, False, XA_CARDINAL,
		&type, &fmt, &n, &bytes, &data) != Success || !data) {
		if (data) XFree(data);
		return 0;
	}
	unsigned int pid = 0;
	if (type == XA_CARDINAL && fmt == 32 && n >= 1) {
		pid = (unsigned int)((unsigned long *)data)[0];
	}
	XFree(data);
	return pid;
}

int window_class(unsigned int xid, char *buf, int buflen) {
	if (!dpy || !xid || !buf || buflen < 2) return -1;
	XClassHint hint;
	hint.res_name = NULL;
	hint.res_class = NULL;
	if (!XGetClassHint(dpy, (Window)xid, &hint)) {
		buf[0] = 0;
		return 0;
	}
	const char *s = hint.res_class ? hint.res_class : "";
	strncpy(buf, s, (size_t)buflen - 1);
	buf[buflen - 1] = 0;
	if (hint.res_name) XFree(hint.res_name);
	if (hint.res_class) XFree(hint.res_class);
	return 0;
}

int window_name(unsigned int xid, char *buf, int buflen) {
	if (!dpy || !xid || !buf || buflen < 2) return -1;
	buf[0] = 0;
	Atom type;
	int fmt;
	unsigned long n = 0, bytes = 0;
	unsigned char *data = NULL;
	Atom name_atom = dest_atoms_ready ? a_net_wm_name : XInternAtom(dpy, "_NET_WM_NAME", False);
	Atom utf8 = dest_atoms_ready ? a_utf8 : XInternAtom(dpy, "UTF8_STRING", False);
	if (XGetWindowProperty(dpy, (Window)xid, name_atom, 0, 64, False, utf8,
		&type, &fmt, &n, &bytes, &data) == Success && data && type == utf8 && fmt == 8 && n > 0) {
		if (n > (unsigned long)buflen - 1) n = (unsigned long)buflen - 1;
		memcpy(buf, data, n);
		buf[n] = 0;
		XFree(data);
		return 0;
	}
	if (data) XFree(data);
	{
		char *nm = NULL;
		if (XFetchName(dpy, (Window)xid, &nm) && nm) {
			strncpy(buf, nm, (size_t)buflen - 1);
			buf[buflen - 1] = 0;
			XFree(nm);
		}
	}
	return 0;
}

int window_has_fullscreen(unsigned int xid) {
	if (!dpy || !dest_atoms_ready || !xid) return 0;
	Atom atoms[32];
	int n = get_atom_list((Window)xid, a_net_wm_state, XA_ATOM, atoms, 32);
	if (n < 0) return 0;
	int i;
	for (i = 0; i < n; i++) {
		if (atoms[i] == a_net_wm_state_fs) return 1;
	}
	return 0;
}

unsigned int get_active_xid(void);

int send_net_active(unsigned int xid) {
	if (!dpy || !dest_atoms_ready || !xid) return -1;
	Window root = DefaultRootWindow(dpy);
	XEvent ev;
	memset(&ev, 0, sizeof(ev));
	ev.xclient.type = ClientMessage;
	ev.xclient.window = (Window)xid;
	ev.xclient.message_type = a_net_active;
	ev.xclient.format = 32;
	ev.xclient.data.l[0] = 2;
	ev.xclient.data.l[1] = CurrentTime;
	ev.xclient.data.l[2] = (long)get_active_xid();
	ev.xclient.data.l[3] = 0;
	ev.xclient.data.l[4] = 0;
	Status ok = XSendEvent(dpy, root, False,
		SubstructureRedirectMask | SubstructureNotifyMask, &ev);
	XSync(dpy, False);
	return ok ? 0 : -1;
}

int send_net_wm_state_fs(unsigned int xid, int add) {
	if (!dpy || !dest_atoms_ready || !xid) return -1;
	Window root = DefaultRootWindow(dpy);
	XEvent ev;
	memset(&ev, 0, sizeof(ev));
	ev.xclient.type = ClientMessage;
	ev.xclient.window = (Window)xid;
	ev.xclient.message_type = a_net_wm_state;
	ev.xclient.format = 32;
	ev.xclient.data.l[0] = add ? 1 : 0;
	ev.xclient.data.l[1] = (long)a_net_wm_state_fs;
	ev.xclient.data.l[2] = 0;
	ev.xclient.data.l[3] = 2;
	ev.xclient.data.l[4] = 0;
	Status ok = XSendEvent(dpy, root, False,
		SubstructureRedirectMask | SubstructureNotifyMask, &ev);
	XSync(dpy, False);
	return ok ? 0 : -1;
}

int set_input_focus(unsigned int xid) {
	if (!dpy || !xid) return -1;
	XSetInputFocus(dpy, (Window)xid, RevertToParent, CurrentTime);
	XSync(dpy, False);
	return 0;
}

unsigned int get_active_xid(void) {
	if (!dpy || !dest_atoms_ready) return 0;
	Atom type;
	int fmt;
	unsigned long n = 0, bytes = 0;
	unsigned char *data = NULL;
	Window root = DefaultRootWindow(dpy);
	if (XGetWindowProperty(dpy, root, a_net_active, 0, 1, False, XA_WINDOW,
		&type, &fmt, &n, &bytes, &data) != Success || !data) {
		if (data) XFree(data);
		return 0;
	}
	unsigned int xid = 0;
	if (type == XA_WINDOW && fmt == 32 && n >= 1) {
		xid = (unsigned int)((Window *)data)[0];
	}
	XFree(data);
	return xid;
}

int query_pointer(int *x, int *y) {
	if (!dpy || !x || !y) return -1;
	Window root = DefaultRootWindow(dpy), rr, child;
	int rx, ry, wx, wy;
	unsigned int mask;
	if (!XQueryPointer(dpy, root, &rr, &child, &rx, &ry, &wx, &wy, &mask)) {
		return -1;
	}
	*x = rx;
	*y = ry;
	return 0;
}

int move_mouse_fake(int x, int y) {
	if (!dpy) return -1;
	if (x < 0) x = 0;
	if (y < 0) y = 0;
	if (x >= screen_w) x = screen_w - 1;
	if (y >= screen_h) y = screen_h - 1;
	XTestFakeMotionEvent(dpy, DefaultScreen(dpy), x, y, 0);
	XSync(dpy, False);
	return 0;
}

void close_display(void) {
	if (!dpy) return;
	XCloseDisplay(dpy);
	dpy = NULL;
	dest_atoms_ready = 0;
}

int peer_ucred(int fd, unsigned int *uid, unsigned int *gid) {
#ifdef SO_PEERCRED
	struct ucred c;
	socklen_t n = sizeof(c);
	if (getsockopt(fd, SOL_SOCKET, SO_PEERCRED, &c, &n) != 0) return -1;
	if (uid) *uid = c.uid;
	if (gid) *gid = c.gid;
	return 0;
#else
	(void)fd; (void)uid; (void)gid;
	return -1;
#endif
}
*/
import "C"

import "unsafe"

func initDisplay() int {
	return int(C.init_display())
}

func closeDisplay() {
	C.close_display()
}

func releaseAll() int {
	return int(C.release_all())
}

func sendKey(keycode uint, press bool) int {
	isPress := C.int(0)
	if press {
		isPress = 1
	}
	return int(C.send_key(C.uint(keycode), isPress))
}

func moveMouse(x, y int) int {
	return int(C.move_mouse(C.int(x), C.int(y)))
}

func sendButton(button uint, press bool) int {
	isPress := C.int(0)
	if press {
		isPress = 1
	}
	return int(C.send_button(C.uint(button), isPress))
}

func keysymToKeycode(name string) uint {
	cs := C.CString(name)
	defer C.free(unsafe.Pointer(cs))
	return uint(C.keysym_to_keycode(cs))
}

func internDestAtoms() int { return int(C.intern_dest_atoms()) }

func netSupportedHas(name string) int {
	cs := C.CString(name)
	defer C.free(unsafe.Pointer(cs))
	return int(C.net_supported_has(cs))
}

func refreshClientList() ([]uint32, int) {
	var buf [256]C.uint
	n := int(C.refresh_client_list(&buf[0], 256))
	if n < 0 {
		return nil, -1
	}
	if n == 0 {
		return nil, 0
	}
	out := make([]uint32, n)
	for i := 0; i < n; i++ {
		out[i] = uint32(buf[i])
	}
	return out, 0
}

func windowGeometry(xid uint32) (x, y, w, h int, err int) {
	var cx, cy, cw, ch C.int
	if C.window_geometry(C.uint(xid), &cx, &cy, &cw, &ch) != 0 {
		return 0, 0, 0, 0, -1
	}
	return int(cx), int(cy), int(cw), int(ch), 0
}

func displayGeometry() (int, int) { return int(C.display_width()), int(C.display_height()) }

func windowPID(xid uint32) uint32 { return uint32(C.window_pid(C.uint(xid))) }

func windowClass(xid uint32) string {
	var buf [256]C.char
	C.window_class(C.uint(xid), &buf[0], 256)
	return C.GoString(&buf[0])
}

func windowName(xid uint32) string {
	var buf [256]C.char
	C.window_name(C.uint(xid), &buf[0], 256)
	return C.GoString(&buf[0])
}

func windowHasFullscreen(xid uint32) bool {
	return C.window_has_fullscreen(C.uint(xid)) != 0
}

func sendNetActive(xid uint32) int { return int(C.send_net_active(C.uint(xid))) }

func sendNetWMStateFS(xid uint32, add bool) int {
	v := C.int(0)
	if add {
		v = 1
	}
	return int(C.send_net_wm_state_fs(C.uint(xid), v))
}

func setInputFocus(xid uint32) int { return int(C.set_input_focus(C.uint(xid))) }

func getActiveXID() uint32 { return uint32(C.get_active_xid()) }

func queryPointer() (x, y int, err int) {
	var cx, cy C.int
	if C.query_pointer(&cx, &cy) != 0 {
		return 0, 0, -1
	}
	return int(cx), int(cy), 0
}

func moveMouseFake(x, y int) int { return int(C.move_mouse_fake(C.int(x), C.int(y))) }

func peerUID(fd int) (uid, gid uint32, ok bool) {
	var u, g C.uint
	if C.peer_ucred(C.int(fd), &u, &g) != 0 {
		return 0, 0, false
	}
	return uint32(u), uint32(g), true
}
