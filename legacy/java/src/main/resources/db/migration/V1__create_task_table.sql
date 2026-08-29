CREATE TABLE task (
                      id BIGINT NOT NULL AUTO_INCREMENT,
                      prompt TEXT NOT NULL,
                      status VARCHAR(32) NOT NULL,
                      created_at DATETIME(6) NOT NULL,
                      updated_at DATETIME(6) NOT NULL,
                      PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;