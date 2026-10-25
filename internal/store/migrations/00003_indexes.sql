-- /status 查询用的索引
-- +goose Up
CREATE INDEX IF NOT EXISTS `idx_song_infos_from_user_id` ON `song_infos`(`from_user_id`);
CREATE INDEX IF NOT EXISTS `idx_song_infos_from_chat_id` ON `song_infos`(`from_chat_id`);
CREATE INDEX IF NOT EXISTS `idx_song_infos_created_at` ON `song_infos`(`created_at`);

-- +goose Down
DROP INDEX IF EXISTS `idx_song_infos_created_at`;
DROP INDEX IF EXISTS `idx_song_infos_from_chat_id`;
DROP INDEX IF EXISTS `idx_song_infos_from_user_id`;
