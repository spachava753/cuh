package gmail

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nalgeon/be"
)

const (
	liveTestFlagEnv  = "GMAIL_LIVE_TEST"
	liveTestToEnv    = "GMAIL_TEST_RECIPIENT"
	pollInterval     = 5 * time.Second
	threadWaitWindow = 2 * time.Minute
)

func TestLiveThreadLifecycle(t *testing.T) {
	if os.Getenv(liveTestFlagEnv) != "1" {
		t.Skipf("set %s=1 to run live Gmail integration tests", liveTestFlagEnv)
	}

	address := strings.TrimSpace(os.Getenv(envGmailAddress))
	appPassword := strings.TrimSpace(os.Getenv(envGmailAppPassword))
	if address == "" || appPassword == "" {
		t.Skipf("set %s and %s to run live Gmail integration tests", envGmailAddress, envGmailAppPassword)
	}

	recipient := strings.TrimSpace(os.Getenv(liveTestToEnv))
	if recipient == "" {
		recipient = address
	}

	_, err := ListThreads("", 25)
	be.Err(t, err, nil)

	subject := fmt.Sprintf("cuh live test %d", time.Now().UnixNano())
	body := fmt.Sprintf("integration test body %d", time.Now().UnixNano())
	be.Err(t, SendEmail(recipient, subject, body), nil)

	createdThread, err := waitForThreadBySubject(subject, threadWaitWindow)
	be.Err(t, err, nil)

	threadID := createdThread.ID
	be.True(t, threadID != "")

	cleanedUp := false
	defer func() {
		if cleanedUp || threadID == "" {
			return
		}
		be.Err(t, DeleteThread(threadID), nil)
	}()

	messages, err := GetThread(threadID)
	be.Err(t, err, nil)
	be.True(t, len(messages) > 0)

	foundSubject := false
	for _, msg := range messages {
		if msg.Subject == subject {
			foundSubject = true
			break
		}
	}
	be.True(t, foundSubject)

	label := fmt.Sprintf("cuh-live-test-%d", time.Now().Unix())
	be.Err(t, AddLabelToThread(threadID, label), nil)
	be.Err(t, waitForLabelState(threadID, label, true, 45*time.Second), nil)

	be.Err(t, RemoveLabelFromThread(threadID, label), nil)
	be.Err(t, waitForLabelState(threadID, label, false, 45*time.Second), nil)

	be.Err(t, DeleteThread(threadID), nil)
	cleanedUp = true

	be.Err(t, waitForThreadDeleted(threadID, 90*time.Second), nil)

	threadsAfter, err := ListThreads("", 250)
	be.Err(t, err, nil)

	stillExists := false
	for _, thread := range threadsAfter {
		if thread.ID == threadID {
			stillExists = true
			break
		}
	}
	be.True(t, !stillExists)
}

func waitForThreadBySubject(subject string, timeout time.Duration) (Thread, error) {
	deadline := time.Now().Add(timeout)
	for {
		threads, err := ListThreads("", 300)
		if err != nil {
			return Thread{}, err
		}

		for _, thread := range threads {
			if thread.Subject == subject {
				return thread, nil
			}
		}

		if time.Now().After(deadline) {
			return Thread{}, fmt.Errorf("thread with subject %q not found within %s", subject, timeout)
		}
		time.Sleep(pollInterval)
	}
}

func waitForLabelState(threadID string, label string, present bool, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		messages, err := GetThread(threadID)
		if err != nil {
			return err
		}

		hasLabel := false
		for _, msg := range messages {
			if containsLabel(msg.Labels, label) {
				hasLabel = true
				break
			}
		}
		if hasLabel == present {
			return nil
		}

		if time.Now().After(deadline) {
			state := "removed"
			if present {
				state = "added"
			}
			return fmt.Errorf("label %q was not %s within %s", label, state, timeout)
		}
		time.Sleep(pollInterval)
	}
}

func waitForThreadDeleted(threadID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		_, err := GetThread(threadID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return nil
			}
			return err
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("thread %s still exists after %s", threadID, timeout)
		}
		time.Sleep(pollInterval)
	}
}

func containsLabel(labels []string, label string) bool {
	for _, existing := range labels {
		if existing == label {
			return true
		}
	}
	return false
}
