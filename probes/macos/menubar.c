// lists every layer-25 (status item) window: owner pid and name, bounds.
#include <CoreGraphics/CoreGraphics.h>
#include <stdio.h>
int main(void) {
	CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionAll, kCGNullWindowID);
	for (CFIndex i = 0; i < CFArrayGetCount(list); i++) {
		CFDictionaryRef w = CFArrayGetValueAtIndex(list, i);
		int layer = 0, pid = 0;
		CFNumberGetValue(CFDictionaryGetValue(w, kCGWindowLayer), kCFNumberIntType, &layer);
		if (layer != 25) { continue; }
		CFNumberGetValue(CFDictionaryGetValue(w, kCGWindowOwnerPID), kCFNumberIntType, &pid);
		char name[256] = "?";
		CFStringRef n = CFDictionaryGetValue(w, kCGWindowOwnerName);
		if (n) { CFStringGetCString(n, name, sizeof name, kCFStringEncodingUTF8); }
		CGRect r = CGRectZero;
		CGRectMakeWithDictionaryRepresentation(CFDictionaryGetValue(w, kCGWindowBounds), &r);
		CFBooleanRef on = CFDictionaryGetValue(w, kCGWindowIsOnscreen);
		printf("pid=%d %-28s onscreen=%d x=%.0f w=%.0f\n", pid, name, on ? CFBooleanGetValue(on) : 0, r.origin.x, r.size.width);
	}
	CFRelease(list);
	return 0;
}
