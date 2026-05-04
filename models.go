package git

import (
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/picosh/git-pr/db"
)

// Type aliases for DB-generated types so the rest of the codebase stays the same.
type (
	Status       = db.Status
	EventData    = db.EventData
	User         = db.AppUser
	Acl          = db.Acl
	Repo         = db.Repo
	PatchRequest = db.PatchRequest
	Patchset     = db.Patchset
	EventLog     = db.EventLog
)

const (
	StatusOpen     = db.StatusOpen
	StatusClosed   = db.StatusClosed
	StatusAccepted = db.StatusAccepted
	StatusReviewed = db.StatusReviewed
)

// Patch extends the DB Patch with parsed diff files.
type Patch struct {
	db.Patch
	Files []*gitdiff.File
}

func (p *Patch) CalcDiff() string {
	return p.RawText
}
