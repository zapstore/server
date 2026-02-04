CREATE TABLE IF NOT EXISTS blobs (
    hash        TEXT    PRIMARY KEY,    -- sha256 stored as a hexadecimal
    type        TEXT    NOT NULL,       -- content type of the blob e.g. text/plain charset=utf-8
    size        INTEGER NOT NULL,
    created_at  INTEGER NOT NULL
);