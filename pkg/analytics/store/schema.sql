CREATE TABLE IF NOT EXISTS impressions (
  app_id        TEXT NOT NULL,
  app_pubkey    TEXT NOT NULL,
  day           DATE NOT NULL,
  source        TEXT NOT NULL,
  type          TEXT NOT NULL,
  country_code  TEXT,
  count         INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (app_id, app_pubkey, day, source, type, country_code)
);

CREATE INDEX IF NOT EXISTS impressions_app_pubkey ON impressions (app_pubkey);
CREATE INDEX IF NOT EXISTS impressions_day ON impressions (day);
CREATE INDEX IF NOT EXISTS impressions_source ON impressions (source);
CREATE INDEX IF NOT EXISTS impressions_type ON impressions (type);
CREATE INDEX IF NOT EXISTS impressions_country_code ON impressions (country_code);

CREATE TABLE IF NOT EXISTS downloads (
  hash          TEXT NOT NULL,
  day           DATE NOT NULL,
  source        TEXT NOT NULL,
  country_code  TEXT,
  count         INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (hash, day, source, country_code)
);

CREATE INDEX IF NOT EXISTS downloads_day ON downloads (day);
CREATE INDEX IF NOT EXISTS downloads_source ON downloads (source);
CREATE INDEX IF NOT EXISTS downloads_country_code ON downloads (country_code);
