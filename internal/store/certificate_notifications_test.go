package store

import (
	"context"
	"testing"
	"time"
)

func TestDeleteNotifyGroupPullsManagedCertificateRefs(t *testing.T) {
	st := &notifyGroupPullStore{
		certs: map[string]ManagedCertificate{
			"mcert_1": {ID: "mcert_1", NotifyGroupIDs: []string{"ntf_1", "ntf_2"}},
			"mcert_2": {ID: "mcert_2", NotifyGroupIDs: []string{"ntf_1"}},
		},
	}
	if err := st.pullNotifyGroupFromCerts(context.Background(), "ntf_1"); err != nil {
		t.Fatal(err)
	}
	if len(st.certs["mcert_1"].NotifyGroupIDs) != 1 || st.certs["mcert_1"].NotifyGroupIDs[0] != "ntf_2" {
		t.Fatalf("mcert_1 notify = %v", st.certs["mcert_1"].NotifyGroupIDs)
	}
	if len(st.certs["mcert_2"].NotifyGroupIDs) != 0 {
		t.Fatalf("mcert_2 notify = %v", st.certs["mcert_2"].NotifyGroupIDs)
	}
}

func TestTryRecordOperationFailureNotificationCAS(t *testing.T) {
	st := &notificationCASStore{certs: map[string]ManagedCertificate{
		"mcert_1": {ID: "mcert_1"},
	}}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	ok, err := st.tryFirstFail("mcert_1", now)
	if err != nil || !ok {
		t.Fatalf("first = %v, %v", ok, err)
	}
	ok, err = st.tryFirstFail("mcert_1", now)
	if err != nil || ok {
		t.Fatalf("second = %v, %v", ok, err)
	}
}

// notifyGroupPullStore mirrors DeleteNotifyGroup managed-cert pull logic in memory.
type notifyGroupPullStore struct {
	certs map[string]ManagedCertificate
}

func (s *notifyGroupPullStore) pullNotifyGroupFromCerts(_ context.Context, id string) error {
	for certID, cert := range s.certs {
		out := cert.NotifyGroupIDs[:0]
		for _, ngID := range cert.NotifyGroupIDs {
			if ngID != id {
				out = append(out, ngID)
			}
		}
		cert.NotifyGroupIDs = out
		s.certs[certID] = cert
	}
	return nil
}

type notificationCASStore struct {
	certs map[string]ManagedCertificate
}

func (s *notificationCASStore) tryFirstFail(certID string, now time.Time) (bool, error) {
	cert, ok := s.certs[certID]
	if !ok {
		return false, nil
	}
	if cert.NotificationState != nil && cert.NotificationState.HadOperationFailure {
		return false, nil
	}
	if cert.NotificationState == nil {
		cert.NotificationState = &CertificateNotificationState{}
	}
	cert.NotificationState.HadOperationFailure = true
	cert.NotificationState.LastFailNotifiedAt = &now
	s.certs[certID] = cert
	return true, nil
}
