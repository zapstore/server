-- Zapstore-specific tag indexing triggers
-- These augment the base nostr-sqlite schema by indexing relevant tags for each event kind.
-- Note: 'd' tags for kinds 30000-39999 are already indexed by the base schema.

-- KindApp (32267) - Software Application
CREATE TRIGGER IF NOT EXISTS app_tags_ai AFTER INSERT ON events
WHEN NEW.kind = 32267
BEGIN
    INSERT OR IGNORE INTO tags (event_id, key, value)
    SELECT NEW.id, json_extract(value, '$[0]'), json_extract(value, '$[1]')
    FROM json_each(NEW.tags)
    WHERE json_type(value) = 'array'
      AND json_array_length(value) > 1
      AND json_extract(value, '$[0]') IN (
        'name', 'f', 'summary', 'icon', 'image', 't', 'url', 'repository', 'license'
      );
END;

-- KindRelease (30063) - Software Release
CREATE TRIGGER IF NOT EXISTS release_tags_ai AFTER INSERT ON events
WHEN NEW.kind = 30063
BEGIN
    INSERT OR IGNORE INTO tags (event_id, key, value)
    SELECT NEW.id, json_extract(value, '$[0]'), json_extract(value, '$[1]')
    FROM json_each(NEW.tags)
    WHERE json_type(value) = 'array'
      AND json_array_length(value) > 1
      AND json_extract(value, '$[0]') IN ('i', 'version', 'c', 'e');
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
      AND json_extract(value, '$[0]') IN (
        'i', 'x', 'version', 'f', 'url', 'm', 'size',
        'min_platform_version', 'target_platform_version', 'supported_nip',
        'filename', 'variant', 'commit', 'min_allowed_version',
        'version_code', 'min_allowed_version_code', 'apk_certificate_hash', 'executable'
      );
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
      AND json_extract(value, '$[0]') IN (
        'x', 'url', 'fallback', 'm', 'version', 'version_code', 'f',
        'apk_signature_hash', 'min_sdk_version', 'target_sdk_version'
      );
END;
