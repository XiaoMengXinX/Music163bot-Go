-- 去重（每组 music_id 保留最新一行）后建立唯一索引，防止并发插入重复缓存
-- +goose Up
DELETE FROM `song_infos`
WHERE `id` NOT IN (SELECT MAX(`id`) FROM `song_infos` GROUP BY `music_id`);
CREATE UNIQUE INDEX IF NOT EXISTS `idx_song_infos_music_id` ON `song_infos`(`music_id`);

-- +goose Down
DROP INDEX IF EXISTS `idx_song_infos_music_id`;
