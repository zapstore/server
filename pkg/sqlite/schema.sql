-- Zapstore schema extensions for tag indexing and full-text search.
-- Note: 'd' tags for kinds 30000-39999 are already indexed by the base schema.

CREATE VIRTUAL TABLE IF NOT EXISTS apps_fts USING fts5(
	id UNINDEXED,
	name,
	summary,
	content,
	tokenize = 'trigram'
);

-- KindApp (32267) - Software Application
CREATE TRIGGER IF NOT EXISTS app_tags_ai AFTER INSERT ON events
WHEN NEW.kind = 32267
BEGIN
	INSERT OR IGNORE INTO tags (event_id, key, value)
	SELECT NEW.id, json_extract(value, '$[0]'), json_extract(value, '$[1]')
	FROM json_each(NEW.tags)
	WHERE json_type(value) = 'array'
		AND json_array_length(value) > 1
		AND json_extract(value, '$[0]') IN ('name', 't', 'f', 'license', 'url', 'repository', 'a'); -- Legacy tags (a)
END;

-- Full-text search index for apps
CREATE TRIGGER IF NOT EXISTS app_fts_ai AFTER INSERT ON events
WHEN NEW.kind = 32267
BEGIN
	INSERT INTO apps_fts (id, name, summary, content)
	VALUES (
		NEW.id,
		(SELECT json_extract(value, '$[1]') FROM json_each(NEW.tags)
			WHERE json_extract(value, '$[0]') = 'name' LIMIT 1),
		(SELECT json_extract(value, '$[1]') FROM json_each(NEW.tags)
			WHERE json_extract(value, '$[0]') = 'summary' LIMIT 1),
		NEW.content
	);
END;

CREATE TRIGGER IF NOT EXISTS app_fts_ad AFTER DELETE ON events
WHEN OLD.kind = 32267
BEGIN
	DELETE FROM apps_fts WHERE id = OLD.id;
END;

-- KindRelease (30063)
CREATE TRIGGER IF NOT EXISTS release_tags_ai AFTER INSERT ON events
WHEN NEW.kind = 30063
BEGIN
	INSERT OR IGNORE INTO tags (event_id, key, value)
	SELECT NEW.id, json_extract(value, '$[0]'), json_extract(value, '$[1]')
	FROM json_each(NEW.tags)
	WHERE json_type(value) = 'array'
		AND json_array_length(value) > 1
		AND json_extract(value, '$[0]') IN ('i', 'version', 'c', 'e', 'a', 'commit'); -- Legacy tags (a, commit)
END;

-- KindAsset (3063) - Software Asset
CREATE TRIGGER IF NOT EXISTS asset_tags_ai AFTER INSERT ON events
WHEN NEW.kind = 3063
BEGIN
	INSERT OR IGNORE INTO tags (event_id, key, value)
	SELECT NEW.id, json_extract(value, '$[0]'), json_extract(value, '$[1]')
	FROM json_each(NEW.tags)
	WHERE json_type(value) = 'array'
		AND json_array_length(value) > 1
		AND json_extract(value, '$[0]') IN ('i', 'x', 'f', 'm', 'url', 'version', 'apk_certificate_hash');
END;

-- KindFile (1063) - Legacy File Metadata
CREATE TRIGGER IF NOT EXISTS file_tags_ai AFTER INSERT ON events
WHEN NEW.kind = 1063
BEGIN
	INSERT OR IGNORE INTO tags (event_id, key, value)
	SELECT NEW.id, json_extract(value, '$[0]'), json_extract(value, '$[1]')
	FROM json_each(NEW.tags)
	WHERE json_type(value) = 'array'
		AND json_array_length(value) > 1
		AND json_extract(value, '$[0]') IN ('x', 'f', 'm', 'url', 'fallback', 'version', 'apk_signature_hash');
END;
