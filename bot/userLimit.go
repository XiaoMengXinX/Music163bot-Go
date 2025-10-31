package bot

import (
	"fmt"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	requiredChannel string // 必须订阅的频道
	dailyLimit      int    // 每日下载限制
)

// initUserLimit 初始化用户限制配置
func initUserLimit(cfg map[string]string) {
	requiredChannel = cfg["RequiredChannel"]
	if requiredChannel == "" {
		requiredChannel = "stardreammusic" // 默认频道
	}

	dailyLimit = 10 // 默认每日限制10次
	if cfg["DailyLimit"] != "" {
		if limit, err := strconv.Atoi(cfg["DailyLimit"]); err == nil {
			dailyLimit = limit
		}
	}
}

// checkChannelSubscription 检查用户是否订阅了频道
func checkChannelSubscription(userID int64) (bool, error) {
	// 如果没有配置必须订阅的频道，则跳过检查
	if requiredChannel == "" {
		return true, nil
	}

	// 检查用户是否是管理员
	for _, adminID := range botAdmin {
		if int64(adminID) == userID {
			return true, nil
		}
	}

	channelUsername := "@" + requiredChannel
	member, err := bot.GetChatMember(tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: channelUsername,
			UserID: userID,
		},
	})
	if err != nil {
		return false, err
	}

	// 检查用户状态
	status := member.Status
	return status == "member" || status == "administrator" || status == "creator", nil
}

// checkDailyLimit 检查用户是否超过每日下载限制
func checkDailyLimit(userID int64, userName string) (bool, int, error) {
	// 管理员不受限制
	for _, adminID := range botAdmin {
		if int64(adminID) == userID {
			return true, 0, nil
		}
	}

	today := time.Now().Format("2006-01-02")
	var userStats UserStats

	// 查询用户统计
	result := UserStatsDB.Where("user_id = ?", userID).First(&userStats)

	if result.Error != nil {
		// 如果用户不存在，创建新记录
		userStats = UserStats{
			UserID:        userID,
			UserName:      userName,
			TotalDownload: 0,
			TodayDownload: 0,
			LastResetDate: today,
		}
		UserStatsDB.Create(&userStats)
	} else {
		// 如果日期不是今天，重置今日下载计数
		if userStats.LastResetDate != today {
			userStats.TodayDownload = 0
			userStats.LastResetDate = today
			UserStatsDB.Save(&userStats)
		}
	}

	// 检查是否超过限制
	if userStats.TodayDownload >= dailyLimit {
		return false, userStats.TodayDownload, nil
	}

	return true, userStats.TodayDownload, nil
}

// incrementUserDownload 增加用户下载计数
func incrementUserDownload(userID int64, userName string) error {
	today := time.Now().Format("2006-01-02")
	var userStats UserStats

	result := UserStatsDB.Where("user_id = ?", userID).First(&userStats)

	if result.Error != nil {
		// 如果用户不存在，创建新记录
		userStats = UserStats{
			UserID:        userID,
			UserName:      userName,
			TotalDownload: 1,
			TodayDownload: 1,
			LastResetDate: today,
		}
		return UserStatsDB.Create(&userStats).Error
	}

	// 如果日期不是今天，重置今日下载计数
	if userStats.LastResetDate != today {
		userStats.TodayDownload = 0
		userStats.LastResetDate = today
	}

	userStats.TodayDownload++
	userStats.TotalDownload++
	userStats.UserName = userName

	return UserStatsDB.Save(&userStats).Error
}

// getUserStats 获取用户统计信息
func getUserStats(userID int64) (*UserStats, error) {
	today := time.Now().Format("2006-01-02")
	var userStats UserStats

	result := UserStatsDB.Where("user_id = ?", userID).First(&userStats)

	if result.Error != nil {
		return nil, result.Error
	}

	// 如果日期不是今天，重置今日下载计数
	if userStats.LastResetDate != today {
		userStats.TodayDownload = 0
		userStats.LastResetDate = today
		UserStatsDB.Save(&userStats)
	}

	return &userStats, nil
}

// sendSubscribeMessage 发送需要订阅频道的消息
func sendSubscribeMessage(chatID int64, messageID int) {
	channelLink := fmt.Sprintf("https://t.me/%s", requiredChannel)
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(needSubscribe, requiredChannel))
	msg.ReplyToMessageID = messageID

	// 创建内联按钮
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(subscribeButton, channelLink),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(checkSubscribe, "check_subscription"),
		),
	)
	msg.ReplyMarkup = keyboard

	bot.Send(msg)
}

// sendDailyLimitMessage 发送每日限制消息
func sendDailyLimitMessage(chatID int64, messageID int, currentCount int) {
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(dailyLimitMsg, currentCount, dailyLimit))
	msg.ReplyToMessageID = messageID
	bot.Send(msg)
}
