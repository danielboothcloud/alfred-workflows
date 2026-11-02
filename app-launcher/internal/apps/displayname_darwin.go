//go:build darwin && cgo

package apps

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>
#include <stdlib.h>

// displayName returns Finder's localised name for path, or NULL.
static char *displayName(const char *path) {
	@autoreleasepool {
		NSString *p = [NSString stringWithUTF8String:path];
		if (p == nil) return NULL;
		NSString *n = [[NSFileManager defaultManager] displayNameAtPath:p];
		if (n == nil || n.length == 0) return NULL;
		return strdup(n.UTF8String);
	}
}
*/
import "C"

import (
	"strings"
	"unsafe"
)

// DisplayName returns the name Finder and Spotlight show for an app bundle,
// e.g. "Voice Memos" for VoiceMemos.app.
func DisplayName(path string) string {
	cs := C.CString(path)
	defer C.free(unsafe.Pointer(cs))
	out := C.displayName(cs)
	if out == nil {
		return stem(path)
	}
	defer C.free(unsafe.Pointer(out))
	name := strings.TrimSuffix(C.GoString(out), ".app")
	if name == "" {
		return stem(path)
	}
	return name
}
