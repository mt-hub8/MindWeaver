CREATE VIRTUAL TABLE chunks_fts USING fts5(
    content,
    content = 'chunks',
    content_rowid = 'row_id',
    tokenize = 'trigram'
);

CREATE TRIGGER chunks_fts_insert AFTER INSERT ON chunks BEGIN
    INSERT INTO chunks_fts(rowid, content) VALUES (new.row_id, new.content);
END;

CREATE TRIGGER chunks_fts_delete AFTER DELETE ON chunks BEGIN
    INSERT INTO chunks_fts(chunks_fts, rowid, content)
    VALUES ('delete', old.row_id, old.content);
END;

CREATE TRIGGER chunks_fts_update AFTER UPDATE OF content ON chunks BEGIN
    INSERT INTO chunks_fts(chunks_fts, rowid, content)
    VALUES ('delete', old.row_id, old.content);
    INSERT INTO chunks_fts(rowid, content) VALUES (new.row_id, new.content);
END;
