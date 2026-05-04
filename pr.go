package git

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/picosh/git-pr/db"
)

var ErrPatchExists = errors.New("patch already exists for patch request")

type PatchsetOp int

const (
	OpNormal PatchsetOp = iota
	OpReview
	OpAccept
	OpClose
)

type GitPatchRequest interface {
	GetUsers() ([]*User, error)
	GetUserByID(userID int64) (*User, error)
	GetUserByName(name string) (*User, error)
	GetUserByPubkey(pubkey string) (*User, error)
	GetRepos() ([]*Repo, error)
	GetRepoByID(repoID int64) (*Repo, error)
	GetRepoByName(user *User, repoName string) (*Repo, error)
	CreateRepo(user *User, repoName string) (*Repo, error)
	DeleteRepo(user *User, repoName string) error
	RegisterUser(pubkey, name string) (*User, error)
	IsBanned(pubkey, ipAddress string) error
	SubmitPatchRequest(repoID int64, userID int64, patchset io.Reader) (*PatchRequest, error)
	SubmitPatchset(prID, userID int64, op PatchsetOp, patchset io.Reader) ([]*Patch, error)
	GetPatchRequestByID(prID int64) (*PatchRequest, error)
	GetPatchRequests() ([]*PatchRequest, error)
	GetPatchRequestsByRepoID(repoID int64) ([]*PatchRequest, error)
	GetPatchRequestsByPubkey(pubkey string) ([]*PatchRequest, error)
	GetPatchsetsByPrID(prID int64) ([]*Patchset, error)
	GetPatchsetByID(patchsetID int64) (*Patchset, error)
	GetLatestPatchsetByPrID(prID int64) (*Patchset, error)
	GetPatchesByPatchsetID(prID int64) ([]*Patch, error)
	UpdatePatchRequestStatus(prID, userID int64, status Status, comment string) error
	UpdatePatchRequestName(prID, userID int64, name string) error
	DeletePatchsetByID(userID, prID int64, patchsetID int64) error
	CreateEventLog(tx *sql.Tx, eventLog EventLog) error
	GetEventLogs() ([]*EventLog, error)
	GetEventLogsByRepoName(user *User, repoName string) ([]*EventLog, error)
	GetEventLogsByPrID(prID int64) ([]*EventLog, error)
	GetEventLogsByUserID(userID int64) ([]*EventLog, error)
	DiffPatchsets(aset *Patchset, bset *Patchset) ([]*RangeDiffOutput, error)
}

type PrCmd struct {
	Backend *Backend
}

var (
	_ GitPatchRequest = PrCmd{}
	_ GitPatchRequest = (*PrCmd)(nil)
)

func (pr PrCmd) IsBanned(pubkey, ipAddress string) error {
	ctx := context.Background()
	acl, err := pr.Backend.Queries.GetAclBanned(ctx, db.GetAclBannedParams{
		Pubkey:    sql.NullString{String: pubkey, Valid: pubkey != ""},
		IpAddress: sql.NullString{String: ipAddress, Valid: ipAddress != ""},
	})
	if len(acl) > 0 {
		return fmt.Errorf("user has been banned")
	}
	return err
}

func (pr PrCmd) GetUsers() ([]*User, error) {
	ctx := context.Background()
	rows, err := pr.Backend.Queries.GetUsers(ctx)
	if err != nil {
		return nil, err
	}
	users := make([]*User, len(rows))
	for i := range rows {
		users[i] = &rows[i]
	}
	return users, nil
}

func (pr PrCmd) GetUserByName(name string) (*User, error) {
	ctx := context.Background()
	user, err := pr.Backend.Queries.GetUserByName(ctx, name)
	return &user, err
}

func (pr PrCmd) GetUserByID(id int64) (*User, error) {
	ctx := context.Background()
	user, err := pr.Backend.Queries.GetUserByID(ctx, id)
	return &user, err
}

func (pr PrCmd) GetUserByPubkey(pubkey string) (*User, error) {
	ctx := context.Background()
	user, err := pr.Backend.Queries.GetUserByPubkey(ctx, pubkey)
	return &user, err
}

func (pr PrCmd) computeUserName(name string) (string, error) {
	ctx := context.Background()
	_, err := pr.Backend.Queries.GetUserByName(ctx, name)
	if err != nil {
		return name, nil
	}
	// collision, generate random number and append
	return fmt.Sprintf("%s%s", name, randSeq(4)), nil
}

func (pr PrCmd) CreateRepo(user *User, repoName string) (*Repo, error) {
	ctx := context.Background()
	repoID, err := pr.Backend.Queries.CreateRepo(ctx, db.CreateRepoParams{
		UserID: user.ID,
		Name:   repoName,
	})
	if err != nil {
		return nil, err
	}
	return pr.GetRepoByID(repoID)
}

func (pr PrCmd) DeleteRepo(user *User, repoName string) error {
	ctx := context.Background()
	return pr.Backend.Queries.DeleteRepo(ctx, db.DeleteRepoParams{
		UserID: user.ID,
		Name:   repoName,
	})
}

func (pr PrCmd) GetRepoByID(repoID int64) (*Repo, error) {
	ctx := context.Background()
	repo, err := pr.Backend.Queries.GetRepoByID(ctx, repoID)
	return &repo, err
}

func (pr PrCmd) GetRepos() ([]*Repo, error) {
	ctx := context.Background()
	rows, err := pr.Backend.Queries.GetRepos(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no repos found")
	}
	repos := make([]*Repo, len(rows))
	for i := range rows {
		repos[i] = &rows[i]
	}
	return repos, nil
}

func (pr PrCmd) GetRepoByName(user *User, repoName string) (*Repo, error) {
	ctx := context.Background()
	var (
		repo db.Repo
		err  error
	)
	if user == nil {
		repo, err = pr.Backend.Queries.GetRepoByName(ctx, repoName)
	} else {
		repo, err = pr.Backend.Queries.GetRepoByNameAndUser(ctx, db.GetRepoByNameAndUserParams{
			UserID: user.ID,
			Name:   repoName,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("repo not found: %s", repoName)
	}
	return &repo, nil
}

func (pr PrCmd) createUser(pubkey, name string) (*User, error) {
	if pubkey == "" {
		return nil, fmt.Errorf("must provide pubkey when creating user")
	}
	if name == "" {
		return nil, fmt.Errorf("must provide user name when creating user")
	}

	userName, err := pr.computeUserName(name)
	if err != nil {
		pr.Backend.Logger.Error("could not compute username", "err", err)
	}

	ctx := context.Background()
	userID, err := pr.Backend.Queries.CreateUser(ctx, db.CreateUserParams{
		Pubkey: pubkey,
		Name:   userName,
	})
	if err != nil {
		return nil, err
	}
	if userID == 0 {
		return nil, fmt.Errorf("could not create user")
	}

	return pr.GetUserByID(userID)
}

func (pr PrCmd) RegisterUser(pubkey, name string) (*User, error) {
	sanName := strings.ToLower(name)
	if pubkey == "" {
		return nil, fmt.Errorf("must provide pubkey during upsert")
	}
	_, err := pr.GetUserByPubkey(pubkey)
	if err == nil {
		return nil, fmt.Errorf("pubkey is already registered by another user")
	}
	return pr.createUser(pubkey, sanName)
}

func (pr PrCmd) GetPatchsetsByPrID(prID int64) ([]*Patchset, error) {
	ctx := context.Background()
	rows, err := pr.Backend.Queries.GetPatchsetsByPrID(ctx, prID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no patchsets found for patch request: %d", prID)
	}
	patchsets := make([]*Patchset, len(rows))
	for i := range rows {
		patchsets[i] = &rows[i]
	}
	return patchsets, nil
}

func (pr PrCmd) GetPatchsetByID(patchsetID int64) (*Patchset, error) {
	ctx := context.Background()
	patchset, err := pr.Backend.Queries.GetPatchsetByID(ctx, patchsetID)
	return &patchset, err
}

func (pr PrCmd) GetLatestPatchsetByPrID(prID int64) (*Patchset, error) {
	patchsets, err := pr.GetPatchsetsByPrID(prID)
	if err != nil {
		return nil, err
	}
	if len(patchsets) == 0 {
		return nil, fmt.Errorf("not patchsets found for patch request: %d", prID)
	}
	return patchsets[len(patchsets)-1], nil
}

func (pr PrCmd) GetPatchesByPatchsetID(patchsetID int64) ([]*Patch, error) {
	ctx := context.Background()
	rows, err := pr.Backend.Queries.GetPatchesByPatchsetID(ctx, patchsetID)
	if err != nil {
		return nil, err
	}
	patches := make([]*Patch, len(rows))
	for i := range rows {
		patches[i] = &Patch{Patch: rows[i]}
	}
	return patches, nil
}

func (cmd PrCmd) GetPatchRequests() ([]*PatchRequest, error) {
	ctx := context.Background()
	rows, err := cmd.Backend.Queries.GetPatchRequests(ctx)
	if err != nil {
		return nil, err
	}
	prs := make([]*PatchRequest, len(rows))
	for i := range rows {
		prs[i] = &rows[i]
	}
	return prs, nil
}

func (cmd PrCmd) GetPatchRequestsByRepoID(repoID int64) ([]*PatchRequest, error) {
	ctx := context.Background()
	rows, err := cmd.Backend.Queries.GetPatchRequestsByRepoID(ctx, repoID)
	if err != nil {
		return nil, err
	}
	prs := make([]*PatchRequest, len(rows))
	for i := range rows {
		prs[i] = &rows[i]
	}
	return prs, nil
}

func (cmd PrCmd) GetPatchRequestsByPubkey(pubkey string) ([]*PatchRequest, error) {
	ctx := context.Background()
	rows, err := cmd.Backend.Queries.GetPatchRequestsByPubkey(ctx, pubkey)
	if err != nil {
		return nil, err
	}
	prs := make([]*PatchRequest, len(rows))
	for i := range rows {
		prs[i] = &rows[i]
	}
	return prs, nil
}

func (cmd PrCmd) GetPatchRequestByID(prID int64) (*PatchRequest, error) {
	ctx := context.Background()
	pr, err := cmd.Backend.Queries.GetPatchRequestByID(ctx, prID)
	return &pr, err
}

// Status types: open, closed, accepted, reviewed.
func (cmd PrCmd) UpdatePatchRequestStatus(prID int64, userID int64, status Status, comment string) error {
	tx, err := cmd.Backend.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	ctx := context.Background()
	qtx := cmd.Backend.Queries.WithTx(tx)

	err = qtx.UpdatePatchRequestStatus(ctx, db.UpdatePatchRequestStatusParams{
		Status: status,
		ID:     prID,
	})
	if err != nil {
		return err
	}

	pr, err := cmd.GetPatchRequestByID(prID)
	if err != nil {
		return err
	}

	err = cmd.CreateEventLog(tx, EventLog{
		UserID:         userID,
		RepoID:         sql.NullInt64{Int64: pr.RepoID, Valid: true},
		PatchRequestID: sql.NullInt64{Int64: prID, Valid: true},
		Event:          "pr_status_changed",
		Data: EventData{
			Status:  status,
			Comment: comment,
		},
	})
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (cmd PrCmd) UpdatePatchRequestName(prID int64, userID int64, name string) error {
	if name == "" {
		return fmt.Errorf("must provide name or text in order to update patch request")
	}

	tx, err := cmd.Backend.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	ctx := context.Background()
	qtx := cmd.Backend.Queries.WithTx(tx)

	err = qtx.UpdatePatchRequestName(ctx, db.UpdatePatchRequestNameParams{
		Name: name,
		ID:   prID,
	})
	if err != nil {
		return err
	}

	pr, err := cmd.GetPatchRequestByID(prID)
	if err != nil {
		return err
	}

	err = cmd.CreateEventLog(tx, EventLog{
		UserID:         userID,
		RepoID:         sql.NullInt64{Int64: pr.RepoID, Valid: true},
		PatchRequestID: sql.NullInt64{Int64: prID, Valid: true},
		Event:          "pr_name_changed",
		Data: EventData{
			Name: name,
		},
	})
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (cmd PrCmd) CreateEventLog(tx *sql.Tx, eventLog EventLog) error {
	ctx := context.Background()
	qtx := cmd.Backend.Queries.WithTx(tx)

	if eventLog.RepoID.Valid && eventLog.PatchRequestID.Valid {
		repoID, err := qtx.GetPatchRequestRepoID(ctx, eventLog.PatchRequestID.Int64)
		if err != nil {
			cmd.Backend.Logger.Error(
				"could not find pr when creating eventLog",
				"err", err,
			)
			return nil
		}
		eventLog.RepoID = sql.NullInt64{Int64: repoID, Valid: true}
	}

	err := qtx.CreateEventLog(ctx, db.CreateEventLogParams{
		UserID:         eventLog.UserID,
		RepoID:         eventLog.RepoID,
		PatchRequestID: eventLog.PatchRequestID,
		PatchsetID:     eventLog.PatchsetID,
		Event:          eventLog.Event,
		Data:           eventLog.Data,
	})
	if err != nil {
		cmd.Backend.Logger.Error(
			"could not create eventLog",
			"err", err,
		)
	}
	return err
}

func (cmd PrCmd) createPatch(ctx context.Context, qtx *db.Queries, patch *Patch) (int64, error) {
	existing, _ := qtx.CheckPatchExists(ctx, db.CheckPatchExistsParams{
		PatchsetID: patch.PatchsetID,
		ContentSha: patch.ContentSha,
	})
	if len(existing) > 0 {
		return 0, ErrPatchExists
	}

	patchID, err := qtx.CreatePatch(ctx, db.CreatePatchParams{
		UserID:        patch.UserID,
		PatchsetID:    patch.PatchsetID,
		AuthorName:    patch.AuthorName,
		AuthorEmail:   patch.AuthorEmail,
		AuthorDate:    patch.AuthorDate,
		Title:         patch.Title,
		Body:          patch.Body,
		BodyAppendix:  patch.BodyAppendix,
		CommitSha:     patch.CommitSha,
		ContentSha:    patch.ContentSha,
		BaseCommitSha: patch.BaseCommitSha,
		RawText:       patch.RawText,
	})
	if err != nil {
		return 0, err
	}
	if patchID == 0 {
		return 0, fmt.Errorf("could not create patch request")
	}
	return patchID, nil
}

func (cmd PrCmd) SubmitPatchRequest(repoID int64, userID int64, patchset io.Reader) (*PatchRequest, error) {
	tx, err := cmd.Backend.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	patches, err := ParsePatchset(patchset)
	if err != nil {
		return nil, err
	}

	if len(patches) == 0 {
		return nil, fmt.Errorf("after parsing patchset we did't find any patches, did you send us an empty patchset?")
	}

	prName := ""
	prText := ""
	if len(patches) > 0 {
		prName = patches[0].Title
		prText = patches[0].Body
	}

	ctx := context.Background()
	qtx := cmd.Backend.Queries.WithTx(tx)

	prID, err := qtx.CreatePatchRequest(ctx, db.CreatePatchRequestParams{
		UserID:    userID,
		RepoID:    repoID,
		Name:      prName,
		Text:      prText,
		Status:    StatusOpen,
		UpdatedAt: time.Now(),
	})
	if err != nil {
		return nil, err
	}
	if prID == 0 {
		return nil, fmt.Errorf("could not create patch request")
	}

	patchsetID, err := qtx.CreatePatchset(ctx, db.CreatePatchsetParams{
		UserID:         userID,
		PatchRequestID: prID,
		Review:         false,
	})
	if err != nil {
		return nil, err
	}
	if patchsetID == 0 {
		return nil, fmt.Errorf("could not create patchset")
	}

	for _, patch := range patches {
		patch.UserID = userID
		patch.PatchsetID = patchsetID
		_, err = cmd.createPatch(ctx, qtx, patch)
		if err != nil {
			return nil, err
		}
	}

	err = cmd.CreateEventLog(tx, EventLog{
		UserID:         userID,
		RepoID:         sql.NullInt64{Int64: repoID, Valid: true},
		PatchRequestID: sql.NullInt64{Int64: prID, Valid: true},
		PatchsetID:     sql.NullInt64{Int64: patchsetID, Valid: true},
		Event:          "pr_created",
	})
	if err != nil {
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return cmd.GetPatchRequestByID(prID)
}

func (cmd PrCmd) SubmitPatchset(prID int64, userID int64, op PatchsetOp, patchset io.Reader) ([]*Patch, error) {
	fin := []*Patch{}
	tx, err := cmd.Backend.DB.Begin()
	if err != nil {
		return fin, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	patches, err := ParsePatchset(patchset)
	if err != nil {
		return fin, err
	}

	ctx := context.Background()
	qtx := cmd.Backend.Queries.WithTx(tx)

	isReview := op == OpReview || op == OpAccept || op == OpClose
	patchsetID, err := qtx.CreatePatchset(ctx, db.CreatePatchsetParams{
		UserID:         userID,
		PatchRequestID: prID,
		Review:         isReview,
	})
	if err != nil {
		return nil, err
	}
	if patchsetID == 0 {
		return nil, fmt.Errorf("could not create patchset")
	}

	for _, patch := range patches {
		patch.UserID = userID
		patch.PatchsetID = patchsetID
		patchID, err := cmd.createPatch(ctx, qtx, patch)
		if err == nil {
			patch.ID = patchID
			fin = append(fin, patch)
		} else {
			if !errors.Is(ErrPatchExists, err) {
				return fin, err
			}
		}
	}

	if len(fin) > 0 {
		event := "pr_patchset_added"
		if op == OpReview {
			event = "pr_reviewed"
		}

		pr, err := cmd.GetPatchRequestByID(prID)
		if err != nil {
			return fin, err
		}

		err = cmd.CreateEventLog(tx, EventLog{
			UserID:         userID,
			RepoID:         sql.NullInt64{Int64: pr.RepoID, Valid: true},
			PatchRequestID: sql.NullInt64{Int64: prID, Valid: true},
			PatchsetID:     sql.NullInt64{Int64: patchsetID, Valid: true},
			Event:          event,
		})
		if err != nil {
			return fin, err
		}
	}

	err = tx.Commit()
	if err != nil {
		return fin, err
	}

	return fin, err
}

func (cmd PrCmd) DeletePatchsetByID(userID int64, prID int64, patchsetID int64) error {
	tx, err := cmd.Backend.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	ctx := context.Background()
	qtx := cmd.Backend.Queries.WithTx(tx)

	err = qtx.DeletePatchsetByID(ctx, patchsetID)
	if err != nil {
		return err
	}

	pr, err := cmd.GetPatchRequestByID(prID)
	if err != nil {
		return err
	}

	err = cmd.CreateEventLog(tx, EventLog{
		UserID:         userID,
		RepoID:         sql.NullInt64{Int64: pr.RepoID, Valid: true},
		PatchRequestID: sql.NullInt64{Int64: prID, Valid: true},
		PatchsetID:     sql.NullInt64{Int64: patchsetID, Valid: true},
		Event:          "pr_patchset_deleted",
	})
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (cmd PrCmd) GetEventLogs() ([]*EventLog, error) {
	ctx := context.Background()
	rows, err := cmd.Backend.Queries.GetEventLogs(ctx)
	if err != nil {
		return nil, err
	}
	eventLogs := make([]*EventLog, len(rows))
	for i := range rows {
		eventLogs[i] = &rows[i]
	}
	return eventLogs, nil
}

func (cmd PrCmd) GetEventLogsByRepoName(user *User, repoName string) ([]*EventLog, error) {
	repo, err := cmd.GetRepoByName(user, repoName)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	rows, err := cmd.Backend.Queries.GetEventLogsByRepoID(ctx, sql.NullInt64{Int64: repo.ID, Valid: true})
	if err != nil {
		return nil, err
	}
	eventLogs := make([]*EventLog, len(rows))
	for i := range rows {
		eventLogs[i] = &rows[i]
	}
	return eventLogs, nil
}

func (cmd PrCmd) GetEventLogsByPrID(prID int64) ([]*EventLog, error) {
	ctx := context.Background()
	rows, err := cmd.Backend.Queries.GetEventLogsByPrID(ctx, sql.NullInt64{Int64: prID, Valid: true})
	if err != nil {
		return nil, err
	}
	eventLogs := make([]*EventLog, len(rows))
	for i := range rows {
		eventLogs[i] = &rows[i]
	}
	return eventLogs, nil
}

func (cmd PrCmd) GetEventLogsByUserID(userID int64) ([]*EventLog, error) {
	ctx := context.Background()
	rows, err := cmd.Backend.Queries.GetEventLogsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	eventLogs := make([]*EventLog, len(rows))
	for i := range rows {
		eventLogs[i] = &rows[i]
	}
	return eventLogs, nil
}

func (cmd PrCmd) DiffPatchsets(prev *Patchset, next *Patchset) ([]*RangeDiffOutput, error) {
	output := []*RangeDiffOutput{}
	patches, err := cmd.GetPatchesByPatchsetID(next.ID)
	if err != nil {
		return output, err
	}

	for idx, patch := range patches {
		patchStr := patch.RawText
		if idx > 0 {
			patchStr = startOfPatch + patch.RawText
		}
		diffFiles, _, err := ParsePatch(patchStr)
		if err != nil {
			continue
		}
		patch.Files = diffFiles
	}

	if prev == nil {
		return output, nil
	}

	prevPatches, err := cmd.GetPatchesByPatchsetID(prev.ID)
	if err != nil {
		return output, fmt.Errorf("cannot get previous patchset patches: %w", err)
	}

	for idx, patch := range prevPatches {
		patchStr := patch.RawText
		if idx > 0 {
			patchStr = startOfPatch + patch.RawText
		}
		diffFiles, _, err := ParsePatch(patchStr)
		if err != nil {
			continue
		}
		patch.Files = diffFiles
	}

	return RangeDiff(prevPatches, patches), nil
}
