package store

// IDs are UUIDv7 strings: unguessable, and their time prefix keeps "newest first" ordering.
// GitHub app and installation ids are GitHub's own numbers. The audit log uses a plain
// counter that is only used for paging.
var migrations = []string{`
CREATE TABLE users (
	id            TEXT PRIMARY KEY,
	username      TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at    INTEGER NOT NULL
);

CREATE TABLE sessions (
	token_hash TEXT PRIMARY KEY,
	user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at INTEGER NOT NULL
);

CREATE TABLE api_tokens (
	id           TEXT PRIMARY KEY,
	user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name         TEXT NOT NULL,
	token_hash   TEXT NOT NULL UNIQUE,
	last_used_at INTEGER,
	created_at   INTEGER NOT NULL
);

CREATE TABLE settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE projects (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL UNIQUE,
	description TEXT NOT NULL DEFAULT '',
	created_at  INTEGER NOT NULL
);

CREATE TABLE git_credentials (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL UNIQUE,
	host       TEXT NOT NULL,
	username   TEXT NOT NULL,
	token      TEXT NOT NULL,
	created_at INTEGER NOT NULL
);

CREATE TABLE github_apps (
	id             INTEGER PRIMARY KEY,
	slug           TEXT NOT NULL,
	name           TEXT NOT NULL,
	owner          TEXT NOT NULL,
	html_url       TEXT NOT NULL,
	client_id      TEXT NOT NULL,
	client_secret  TEXT NOT NULL,
	private_key    TEXT NOT NULL,
	webhook_secret TEXT NOT NULL,
	created_at     INTEGER NOT NULL
);

CREATE TABLE github_installations (
	id            INTEGER PRIMARY KEY,
	github_app_id INTEGER NOT NULL REFERENCES github_apps(id) ON DELETE CASCADE,
	account       TEXT NOT NULL,
	account_type  TEXT NOT NULL,
	created_at    INTEGER NOT NULL
);

CREATE TABLE apps (
	id                     TEXT PRIMARY KEY,
	project_id             TEXT REFERENCES projects(id),
	name                   TEXT NOT NULL UNIQUE,
	runtime                TEXT NOT NULL DEFAULT '',
	port                   INTEGER NOT NULL DEFAULT 0,
	source                 TEXT NOT NULL DEFAULT 'upload',
	git_url                TEXT NOT NULL DEFAULT '',
	git_branch             TEXT NOT NULL DEFAULT '',
	git_subdir             TEXT NOT NULL DEFAULT '',
	git_credential_id      TEXT REFERENCES git_credentials(id) ON DELETE SET NULL,
	github_installation_id INTEGER REFERENCES github_installations(id) ON DELETE SET NULL,
	autodeploy             INTEGER NOT NULL DEFAULT 1,
	pushed_commit          TEXT NOT NULL DEFAULT '',
	pushed_message         TEXT NOT NULL DEFAULT '',
	webhook_secret         TEXT NOT NULL,
	run_as_user            TEXT NOT NULL DEFAULT '',
	run_as_password        TEXT NOT NULL DEFAULT '',
	keep_bundles           INTEGER NOT NULL DEFAULT 5,
	log_retention_days     INTEGER NOT NULL DEFAULT 14,
	auto_restart           INTEGER NOT NULL DEFAULT 0,
	backup_cron            TEXT NOT NULL DEFAULT '',
	backup_keep            INTEGER NOT NULL DEFAULT 7,
	record_requests        INTEGER NOT NULL DEFAULT 0,
	current_release_id     TEXT NOT NULL DEFAULT '',
	desired_state          TEXT NOT NULL DEFAULT 'stopped',
	created_at             INTEGER NOT NULL
);

CREATE TABLE env_vars (
	app_id TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
	key    TEXT NOT NULL,
	value  TEXT NOT NULL,
	PRIMARY KEY (app_id, key)
);

CREATE TABLE domains (
	id     TEXT PRIMARY KEY,
	app_id TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
	host   TEXT NOT NULL,
	path   TEXT NOT NULL DEFAULT '/',
	tls    TEXT NOT NULL DEFAULT 'none',
	UNIQUE (host, path)
);

CREATE TABLE bundles (
	id         TEXT PRIMARY KEY,
	app_id     TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
	source     TEXT NOT NULL,
	git_commit TEXT NOT NULL DEFAULT '',
	size       INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL
);

CREATE TABLE releases (
	id         TEXT PRIMARY KEY,
	app_id     TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
	bundle_id  TEXT NOT NULL,
	manifest   TEXT NOT NULL DEFAULT 'null',
	image      TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);

CREATE TABLE deployments (
	id          TEXT PRIMARY KEY,
	app_id      TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
	bundle_id   TEXT NOT NULL DEFAULT '',
	release_id  TEXT NOT NULL DEFAULT '',
	trigger     TEXT NOT NULL,
	git_commit  TEXT NOT NULL DEFAULT '',
	git_message TEXT NOT NULL DEFAULT '',
	status      TEXT NOT NULL,
	error       TEXT NOT NULL DEFAULT '',
	created_at  INTEGER NOT NULL,
	started_at  INTEGER,
	finished_at INTEGER
);
CREATE INDEX deployments_app ON deployments(app_id, id);

CREATE TABLE incidents (
	id      TEXT PRIMARY KEY,
	app_id  TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
	down_at INTEGER NOT NULL,
	up_at   INTEGER,
	reason  TEXT NOT NULL
);
CREATE INDEX incidents_app ON incidents(app_id, down_at);

CREATE TABLE uptime (
	app_id TEXT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
	hour   INTEGER NOT NULL,
	ok     INTEGER NOT NULL,
	total  INTEGER NOT NULL,
	PRIMARY KEY (app_id, hour)
) WITHOUT ROWID;

CREATE TABLE backups (
	id         TEXT PRIMARY KEY,
	app_id     TEXT REFERENCES apps(id) ON DELETE CASCADE,
	file       TEXT NOT NULL,
	size       INTEGER NOT NULL DEFAULT 0,
	status     TEXT NOT NULL,
	error      TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);

CREATE TABLE audit_log (
	seq         INTEGER PRIMARY KEY AUTOINCREMENT,
	ts          INTEGER NOT NULL,
	actor_type  TEXT NOT NULL,
	actor_id    TEXT NOT NULL DEFAULT '',
	actor_name  TEXT NOT NULL,
	ip          TEXT NOT NULL DEFAULT '',
	action      TEXT NOT NULL,
	target_type TEXT NOT NULL DEFAULT '',
	target_id   TEXT NOT NULL DEFAULT '',
	target_name TEXT NOT NULL DEFAULT '',
	details     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX audit_log_ts ON audit_log(ts);
`}
