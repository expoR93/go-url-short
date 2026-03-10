-- name: CreateURL :one
INSERT INTO urls(original_url, short_key)
VALUES($1,$2)
RETURNING *;

-- name: GetURL :one
SELECT * FROM urls
WHERE short_key = $1
LIMIT 1;

-- name: IncrementClick :exec
UPDATE urls
SET clicks = clicks + 1
WHERE id = $1;
