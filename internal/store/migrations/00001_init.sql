-- 与 v2 的 song_infos 表结构完全一致；已存在的 v2 库不会受影响
-- +goose Up
CREATE TABLE IF NOT EXISTS `song_infos` (
    `id` integer PRIMARY KEY AUTOINCREMENT,
    `created_at` datetime NULL,
    `updated_at` datetime NULL,
    `deleted_at` datetime NULL,
    `music_id` integer,
    `song_name` text,
    `song_artists` text,
    `song_album` text,
    `file_ext` text,
    `music_size` integer,
    `pic_size` integer,
    `emb_pic_size` integer,
    `bit_rate` integer,
    `duration` integer,
    `file_id` text,
    `thumb_file_id` text,
    `from_user_id` integer,
    `from_user_name` text,
    `from_chat_id` integer,
    `from_chat_name` text
);

-- +goose Down
DROP TABLE IF EXISTS `song_infos`;
