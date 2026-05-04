-- name: GetUsers :many
SELECT * FROM app_users;

-- name: GetUserByID :one
SELECT * FROM app_users WHERE id = ?;

-- name: GetUserByName :one
SELECT * FROM app_users WHERE name = ?;

-- name: GetUserByPubkey :one
SELECT * FROM app_users WHERE pubkey = ?;

-- name: CreateUser :one
INSERT INTO app_users (pubkey, name) VALUES (?, ?) RETURNING id;

-- name: GetRepos :many
SELECT * FROM repos;

-- name: GetRepoByID :one
SELECT * FROM repos WHERE id = ?;

-- name: GetRepoByName :one
SELECT * FROM repos WHERE name = ?;

-- name: GetRepoByNameAndUser :one
SELECT * FROM repos WHERE user_id = ? AND name = ?;

-- name: CreateRepo :one
INSERT INTO repos (user_id, name) VALUES (?, ?) RETURNING id;

-- name: DeleteRepo :exec
DELETE FROM repos WHERE user_id = ? AND name = ?;

-- name: GetAclBanned :many
SELECT * FROM acl WHERE permission = 'banned' AND (pubkey = ? OR ip_address = ?);

-- name: GetPatchRequests :many
SELECT * FROM patch_requests ORDER BY id DESC;

-- name: GetPatchRequestsByRepoID :many
SELECT * FROM patch_requests WHERE repo_id = ? ORDER BY id DESC;

-- name: GetPatchRequestsByPubkey :many
SELECT pr.* FROM patch_requests pr, app_users au WHERE pr.user_id = au.id AND au.pubkey = ? ORDER BY pr.id DESC;

-- name: GetPatchRequestByID :one
SELECT * FROM patch_requests WHERE id = ? ORDER BY created_at DESC;

-- name: GetPatchRequestRepoID :one
SELECT repo_id FROM patch_requests WHERE id = ?;

-- name: CreatePatchRequest :one
INSERT INTO patch_requests (user_id, repo_id, name, text, status, updated_at) VALUES (?, ?, ?, ?, ?, ?) RETURNING id;

-- name: UpdatePatchRequestStatus :exec
UPDATE patch_requests SET status = ? WHERE id = ?;

-- name: UpdatePatchRequestName :exec
UPDATE patch_requests SET name = ? WHERE id = ?;

-- name: GetPatchsetsByPrID :many
SELECT * FROM patchsets WHERE patch_request_id = ? ORDER BY created_at ASC;

-- name: GetPatchsetByID :one
SELECT * FROM patchsets WHERE id = ?;

-- name: CreatePatchset :one
INSERT INTO patchsets (user_id, patch_request_id, review) VALUES (?, ?, ?) RETURNING id;

-- name: DeletePatchsetByID :exec
DELETE FROM patchsets WHERE id = ?;

-- name: GetPatchesByPatchsetID :many
SELECT * FROM patches WHERE patchset_id = ? ORDER BY created_at ASC, id ASC;

-- name: CheckPatchExists :many
SELECT * FROM patches WHERE patchset_id = ? AND content_sha = ?;

-- name: CreatePatch :one
INSERT INTO patches (user_id, patchset_id, author_name, author_email, author_date, title, body, body_appendix, commit_sha, content_sha, base_commit_sha, raw_text) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id;

-- name: CreateEventLog :exec
INSERT INTO event_logs (user_id, repo_id, patch_request_id, patchset_id, event, data) VALUES (?, ?, ?, ?, ?, ?);

-- name: GetEventLogs :many
SELECT * FROM event_logs ORDER BY created_at DESC;

-- name: GetEventLogsByRepoID :many
SELECT * FROM event_logs WHERE repo_id = ? ORDER BY created_at DESC;

-- name: GetEventLogsByPrID :many
SELECT * FROM event_logs WHERE patch_request_id = ? ORDER BY created_at DESC;

-- name: GetEventLogsByUserID :many
SELECT * FROM event_logs
WHERE event_logs.user_id = sqlc.arg(user_id)
    OR event_logs.patch_request_id IN (
        SELECT id FROM patch_requests WHERE patch_requests.user_id = sqlc.arg(user_id)
    )
ORDER BY created_at DESC;
