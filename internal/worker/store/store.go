// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package store supports permanent data storage for the vuln worker.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/vuln/internal/cveschema"
)

// A CVERecord contains information about a CVE.
type CVERecord struct {
	// ID is the CVE ID, which is the same as the filename base. E.g. "CVE-2020-0034".
	ID string
	// Path is the path to the CVE file in the repo.
	Path string
	// Blobhash is the hash of the CVE's blob in repo, for quick change detection.
	BlobHash string
	// CommitHash is the commit of the cvelist repo from which this information came.
	CommitHash string
	// CVEState is the value of the metadata.STATE field.
	CVEState string
	// TriageState is the state of our triage processing on the CVE.
	TriageState TriageState
	// TriageStateReason is an explanation of TriageState.
	TriageStateReason string

	// IssueReference is a reference to the GitHub issue that was filed.
	// E.g. golang/vulndb#12345.
	// Set only after a GitHub issue has been successfully created.
	IssueReference string

	// IssueCreatedAt is the time when the issue was created.
	// Set only after a GitHub issue has been successfully created.
	IssueCreatedAt time.Time
}

// Validate returns an error if the CVERecord is not valid.
func (r *CVERecord) Validate() error {
	if r.ID == "" {
		return errors.New("need ID")
	}
	if r.Path == "" {
		return errors.New("need Path")
	}
	if r.BlobHash == "" {
		return errors.New("need BlobHash")
	}
	if r.CommitHash == "" {
		return errors.New("need CommitHash")
	}
	return r.TriageState.Validate()
}

// TriageState is the state of our work on the CVE.
// It is implemented as a string rather than an int so that stored values are
// immune to renumbering.
type TriageState string

const (
	// No action is needed on the CVE (perhaps because it is rejected, reserved or invalid).
	TriageStateNoActionNeeded TriageState = "NoActionNeeded"
	// The CVE needs to have an issue created.
	TriageStateNeedsIssue TriageState = "NeedsIssue"
	// An issue has been created in the issue tracker.
	// The IssueReference and IssueCreatedAt fields have more information.
	TriageStateIssueCreated TriageState = "IssueCreated"
	// The CVE state was changed after the CVE was created.
	TriageStateUpdatedSinceIssueCreation TriageState = "UpdatedSinceIssueCreation"
)

// Validate returns an error if the TriageState is not one of the above values.
func (s TriageState) Validate() error {
	if s == TriageStateNoActionNeeded || s == TriageStateNeedsIssue || s == TriageStateIssueCreated || s == TriageStateUpdatedSinceIssueCreation {
		return nil
	}
	return fmt.Errorf("bad TriageState %q", s)
}

// NewCVERecord creates a CVERecord from a CVE, its path and its blob hash.
func NewCVERecord(cve *cveschema.CVE, path, blobHash string) *CVERecord {
	return &CVERecord{
		ID:       cve.ID,
		CVEState: cve.State,
		Path:     path,
		BlobHash: blobHash,
	}
}

// A CommitUpdateRecord describes a single update operation, which reconciles
// a commit in the CVE list repo with the DB state.
type CommitUpdateRecord struct {
	// The ID of this record in the DB. Needed to modify the record.
	ID string
	// When the update started and completed. If EndedAt is zero,
	// the update is in progress (or it crashed).
	StartedAt, EndedAt time.Time
	// The repo commit hash that this update is working on.
	CommitHash string
	// The total number of CVEs being processed in this update.
	NumTotal int
	// The number currently processed. When this equals NumTotal, the
	// update is done.
	NumProcessed int
	// The number of CVEs added to the DB.
	NumAdded int
	// The number of CVEs modified.
	NumModified int
	// The error that stopped the update.
	Error string
	// The last time this record was updated.
	UpdatedAt time.Time `firestore:",serverTimestamp"`
}

// A Store is a storage system for the CVE database.
type Store interface {
	// CreateCommitUpdateRecord creates a new CommitUpdateRecord. It should be called at the start
	// of an update. On successful return, the CommitUpdateRecord's ID field will be
	// set to a new, unique ID.
	CreateCommitUpdateRecord(context.Context, *CommitUpdateRecord) error

	// SetCommitUpdateRecord modifies the CommitUpdateRecord. Use the same record passed to
	// CreateCommitUpdateRecord, because it will have the correct ID.
	SetCommitUpdateRecord(context.Context, *CommitUpdateRecord) error

	// ListCommitUpdateRecords returns all the CommitUpdateRecords in the store, from most to
	// least recent.
	ListCommitUpdateRecords(context.Context) ([]*CommitUpdateRecord, error)

	// RunTransaction runs the function in a transaction.
	RunTransaction(context.Context, func(context.Context, Transaction) error) error
}

// Transaction supports store operations that run inside a transaction.
type Transaction interface {
	// CreateCVERecord creates a new CVERecord. It is an error if one with the same ID
	// already exists.
	CreateCVERecord(*CVERecord) error

	// SetCVERecord sets the CVE record in the database. It is
	// an error if no such record exists.
	SetCVERecord(r *CVERecord) error

	// GetCVERecords retrieves CVERecords for all CVE IDs between startID and
	// endID, inclusive.
	GetCVERecords(startID, endID string) ([]*CVERecord, error)
}