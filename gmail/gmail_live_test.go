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
	liveTestFlagEnv = "GMAIL_LIVE_TEST"
	liveTestToEnv   = "GMAIL_TEST_RECIPIENT"

	pollInterval = 5 * time.Second
	waitTimeout  = 2 * time.Minute
)

func TestLivePrimitiveLifecycle(t *testing.T) {
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

	_, err := Labels()
	be.Err(t, err, nil)

	_, err = Find(FindInput{
		Entity:      EntityMessage,
		Page:        Page{Limit: 10},
		Sort:        Sort{By: SortByDate, Order: SortOrderDesc},
		IncludeMeta: true,
	})
	be.Err(t, err, nil)

	subject := fmt.Sprintf("cuh primitive live test %d", time.Now().UnixNano())
	body := fmt.Sprintf("integration test body %d", time.Now().UnixNano())
	sendOut, err := Send(SendInput{
		Message: OutgoingMessage{
			To:       []string{recipient},
			Subject:  subject,
			TextBody: body,
		},
	})
	be.Err(t, err, nil)
	be.True(t, sendOut.MessageID != "")

	ref, err := waitForMessageRefBySubject(subject, waitTimeout)
	be.Err(t, err, nil)
	be.True(t, ref.ID != "")

	cleanupRef := ref
	defer func() {
		if cleanupRef.ID == "" {
			return
		}
		_, _ = Mutate(MutateInput{
			Refs: []Ref{cleanupRef},
			Ops:  []MutationOp{{Type: MutationTrash}},
		})
	}()

	getOut, err := Get(GetInput{
		Refs: []Ref{ref},
		Body: BodyOptions{IncludeText: true, MaxChars: 2000},
	})
	be.Err(t, err, nil)
	be.True(t, len(getOut.Items) > 0)

	foundSubject := false
	for _, item := range getOut.Items {
		if item.Subject == subject {
			foundSubject = true
			break
		}
	}
	be.True(t, foundSubject)

	label := fmt.Sprintf("cuh-live-test-%d", time.Now().Unix())
	mutOut, err := Mutate(MutateInput{
		Refs: []Ref{ref},
		Ops:  []MutationOp{{Type: MutationAddLabel, Value: label}},
	})
	be.Err(t, err, nil)
	be.True(t, mutationSucceeded(mutOut))
	be.Err(t, waitForLabel(ref, label, true, waitTimeout), nil)

	mutOut, err = Mutate(MutateInput{
		Refs: []Ref{ref},
		Ops:  []MutationOp{{Type: MutationRemoveLabel, Value: label}},
	})
	be.Err(t, err, nil)
	be.True(t, mutationSucceeded(mutOut))
	be.Err(t, waitForLabel(ref, label, false, waitTimeout), nil)

	mutOut, err = Mutate(MutateInput{
		Refs: []Ref{ref},
		Ops:  []MutationOp{{Type: MutationTrash}},
	})
	be.Err(t, err, nil)
	be.True(t, mutationSucceeded(mutOut))

	be.Err(t, waitForMessageRemovedFromAllMail(subject, waitTimeout), nil)
	cleanupRef = Ref{}
}

func mutationSucceeded(out MutateOutput) bool {
	if len(out.Results) == 0 {
		return false
	}
	for _, result := range out.Results {
		if !result.Succeeded {
			return false
		}
	}
	return true
}

func waitForMessageRefBySubject(subject string, timeout time.Duration) (Ref, error) {
	deadline := time.Now().Add(timeout)
	for {
		findOut, err := Find(FindInput{
			Entity: EntityMessage,
			Query: Query{
				SubjectContains: []string{subject},
				InMailbox:       []string{gmailAllMail},
			},
			Page:        Page{Limit: 25},
			Sort:        Sort{By: SortByDate, Order: SortOrderDesc},
			IncludeMeta: true,
		})
		if err != nil {
			return Ref{}, err
		}
		for i := range findOut.Refs {
			if i < len(findOut.Meta) && findOut.Meta[i].Subject == subject {
				return findOut.Refs[i], nil
			}
		}
		if time.Now().After(deadline) {
			return Ref{}, fmt.Errorf("message with subject %q not found in %s", subject, timeout)
		}
		time.Sleep(pollInterval)
	}
}

func waitForLabel(ref Ref, label string, present bool, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		out, err := Get(GetInput{Refs: []Ref{ref}})
		if err != nil {
			return err
		}
		hasLabel := false
		for _, item := range out.Items {
			if containsLabel(item.Labels, label) {
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

func waitForMessageRemovedFromAllMail(subject string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		out, err := Find(FindInput{
			Entity: EntityMessage,
			Query: Query{
				SubjectContains: []string{subject},
				InMailbox:       []string{gmailAllMail},
			},
			Page:        Page{Limit: 20},
			IncludeMeta: true,
		})
		if err != nil {
			return err
		}

		stillPresent := false
		for i := range out.Refs {
			if i < len(out.Meta) && out.Meta[i].Subject == subject {
				stillPresent = true
				break
			}
		}
		if !stillPresent {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("message with subject %q still present in all mail after %s", subject, timeout)
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
