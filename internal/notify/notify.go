// Package notify dispatches OS-level notifications for PR state changes,
// gated by the user's per-event preferences in config.NotificationConfig.
package notify

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

@interface GHNotifyDelegate : NSObject <NSUserNotificationCenterDelegate>
@end

@implementation GHNotifyDelegate
- (void)userNotificationCenter:(NSUserNotificationCenter *)center
        didActivateNotification:(NSUserNotification *)notification {
    NSString *u = notification.userInfo[@"url"];
    if (u.length > 0) {
        NSURL *nsurl = [NSURL URLWithString:u];
        if (nsurl) {
            [[NSWorkspace sharedWorkspace] openURL:nsurl];
        }
    }
    [center removeDeliveredNotification:notification];
}

- (BOOL)userNotificationCenter:(NSUserNotificationCenter *)center
     shouldPresentNotification:(NSUserNotification *)notification {
    return YES;
}
@end

static GHNotifyDelegate *gDelegate = nil;

static void ghnEnsureDelegate(void) {
    if (gDelegate == nil) {
        gDelegate = [[GHNotifyDelegate alloc] init];
    }
    NSUserNotificationCenter *c = [NSUserNotificationCenter defaultUserNotificationCenter];
    if (c.delegate != gDelegate) {
        c.delegate = gDelegate;
    }
}

static void ghnDeliver(const char *title, const char *body, const char *url) {
    NSString *t = title != NULL ? [NSString stringWithUTF8String:title] : @"";
    NSString *b = body != NULL  ? [NSString stringWithUTF8String:body]  : @"";
    NSString *u = (url != NULL && url[0] != '\0') ? [NSString stringWithUTF8String:url] : nil;

    dispatch_async(dispatch_get_main_queue(), ^{
        ghnEnsureDelegate();
        NSUserNotification *n = [[NSUserNotification alloc] init];
        n.title = t;
        n.informativeText = b;
        if (u != nil) {
            n.userInfo = @{@"url": u};
        }
        n.identifier = [[NSUUID UUID] UUIDString];
        [[NSUserNotificationCenter defaultUserNotificationCenter] deliverNotification:n];
    });
}
*/
import "C"

import (
	"fmt"
	"log"
	"unsafe"

	"github.com/boyvinall/ghnotify/internal/config"
	"github.com/boyvinall/ghnotify/internal/github"
	"github.com/boyvinall/ghnotify/internal/poller"
)

// Notifier fires OS notifications for PR changes that the user has opted into.
type Notifier struct {
	cfg    *config.NotificationConfig
	snooze *poller.SnoozeStore
}

// NewNotifier creates a Notifier backed by cfg and snooze. cfg may be mutated
// at runtime (e.g., from Preferences) as long as reads are not concurrent with
// writes; for v1 this is safe because prefs are only changed at app restart.
func NewNotifier(cfg *config.NotificationConfig, snooze *poller.SnoozeStore) *Notifier {
	return &Notifier{cfg: cfg, snooze: snooze}
}

// HandleChanges processes a batch of changes from one poll cycle.
func (n *Notifier) HandleChanges(changes []poller.Change) {
	for _, c := range changes {
		n.dispatch(c)
	}
}

func (n *Notifier) dispatch(c poller.Change) {
	if n.snooze.IsSnoozed(c.PR) {
		return
	}
	title, body, ok := n.format(c)
	if !ok {
		return
	}
	if err := osNotify(title, body, c.PR.URL); err != nil {
		log.Printf("ghnotify: notification: %v", err)
	}
}

// osNotify delivers a desktop notification via NSUserNotificationCenter that,
// when clicked, opens url in the default browser.
func osNotify(title, body, url string) error {
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cTitle))
	cBody := C.CString(body)
	defer C.free(unsafe.Pointer(cBody))
	cURL := C.CString(url)
	defer C.free(unsafe.Pointer(cURL))
	C.ghnDeliver(cTitle, cBody, cURL)
	return nil
}

func (n *Notifier) format(c poller.Change) (title, body string, ok bool) {
	pr := c.PR
	repo := fmt.Sprintf("%s › %s/%s", pr.Server, pr.Owner, pr.Repo)

	switch c.Kind {
	case poller.ChangeAdded:
		if c.IsReview && n.cfg.NewReviewRequests {
			return repo,
				fmt.Sprintf("Review requested on #%d: %s", pr.Number, pr.Title),
				true
		}

	case poller.ChangeRemoved:
		// ChangeRemoved on my PRs means merged or closed.
		if !c.IsReview && n.cfg.PRMerged {
			return repo,
				fmt.Sprintf("#%d closed/merged: %s", pr.Number, pr.Title),
				true
		}

	case poller.ChangeReview:
		if !c.IsReview && n.cfg.PRApproved {
			switch pr.ReviewState {
			case github.ReviewApproved:
				return repo,
					fmt.Sprintf("#%d approved ✓ — %s", pr.Number, pr.Title),
					true
			case github.ReviewChangesRequested:
				return repo,
					fmt.Sprintf("#%d changes requested — %s", pr.Number, pr.Title),
					true
			}
		}

	case poller.ChangeCIStatus:
		if n.cfg.CIStatusChange {
			return repo,
				fmt.Sprintf("#%d CI %s — %s", pr.Number, ciLabel(pr.CIStatus), pr.Title),
				true
		}

	case poller.ChangeComments:
		if n.cfg.NewComments {
			return repo,
				fmt.Sprintf("#%d new activity — %s", pr.Number, pr.Title),
				true
		}
	}
	return "", "", false
}

func ciLabel(s github.CIStatus) string {
	switch s {
	case github.CIPassing:
		return "passing ✓"
	case github.CIFailing:
		return "failing ✗"
	case github.CIPending:
		return "pending ○"
	default:
		return "unknown"
	}
}
