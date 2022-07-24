package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/XiaoMengXinX/Music163bot-Go/v3/symbols"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
	"github.com/traefik/yaegi/stdlib/syscall"
	"github.com/traefik/yaegi/stdlib/unrestricted"
	"github.com/traefik/yaegi/stdlib/unsafe"
)

var config map[string]string
var Register FuncRegister

//todo: 自动创建命令列表及帮助
var commandList map[string]string
var commandHelp map[string]string

var (
	configPath  *string
	isLocalMode *bool
	modulePath  *string
)

var (
	runtimeVer  = fmt.Sprintf(runtime.Version())                     // 编译环境
	versionName = ""                                                 // 程序版本
	versionCode = ""                                                 // 版本号
	commitSHA   = ""                                                 // 编译哈希
	buildTime   = ""                                                 // 编译日期
	buildArch   = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH) // 运行环境
	repoPath    = ""                                                 // 项目地址
)

// LogFormatter 自定义 log 格式
type LogFormatter struct{}

// Format 自定义 log 格式
func (s *LogFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	timestamp := time.Now().Local().Format("2006/01/02 15:04:05")
	var msg string
	msg = fmt.Sprintf("%s [%s] %s (%s:%d)\n", timestamp, strings.ToUpper(entry.Level.String()), entry.Message, path.Base(entry.Caller.File), entry.Caller.Line)
	return []byte(msg), nil
}

func init() {
	Register.Commands = make(FuncCommands)
	f := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	configPath = f.String("c", "config.ini", "config file")
	modulePath = f.String("m", "./modules", "module path")
	isLocalMode = f.Bool("l", false, "local mode")
	_ = f.Parse(os.Args[1:])
	logrus.SetFormatter(&logrus.TextFormatter{
		DisableColors:          false,
		FullTimestamp:          true,
		DisableLevelTruncation: true,
		PadLevelText:           true,
	})
	logrus.SetFormatter(new(LogFormatter))
	logrus.SetReportCaller(true)
	dirExists("./log")
	timeStamp := time.Now().Local().Format("2006-01-02")
	logFile := fmt.Sprintf("./log/%v.log", timeStamp)
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		logrus.Errorln(err)
	}
	output := io.MultiWriter(os.Stdout, file)
	logrus.SetOutput(output)
	config, err = readConfig(*configPath)
	if config["BOT_TOKEN"] == "" {
		logrus.Fatal("Please set BOT_TOKEN in config file")
	}
	if err != nil {
		logrus.Fatal("Failed to read config file:", err)
	}
	if config["LogLevel"] != "" {
		level, err := logrus.ParseLevel(config["LogLevel"])
		if err != nil {
			logrus.Errorln(err)
		} else {
			logrus.SetLevel(level)
		}
	}
	logrus.Printf("Music163bot-Go %s(%s)", versionName, versionCode)
}

func main() {
	for {
		config, err := readConfig(*configPath)
		if err != nil {
			logrus.Fatal("Failed to read config file:", err)
		}
		initConfig(config)
		content, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", filepath.Dir(*configPath), "modules.json"))
		if err != nil {
			logrus.Fatalf("Failed to read modules.json: %s", err)
		}
		modules := &Modules{}
		err = json.Unmarshal(content, modules)
		i := newInterpreter()
		if !*isLocalMode {
			for i := 0; i < len(modules.Modules); i++ {
				err := getModuleUpdate(&modules.Modules[i])
				if err != nil {
					logrus.Errorln(err)
				}
			}
		}
		for _, module := range modules.Modules {
			moduleInfo, err := getLocalModuleInfo(module)
			if err != nil {
				logrus.Errorln(err)
				continue
			}
			commandList, commandHelp = parseCommands(*moduleInfo)
			err = loadModule(i, *moduleInfo)
			if err != nil {
				logrus.Errorln(err)
				continue
			}
			err = registerFunc(i, *moduleInfo)
			if err != nil {
				logrus.Errorln(err)
				continue
			}
		}
		switch startBot(i) {
		case 0:
			logrus.Println("Reloading")
			continue
		}
	}
}

func startBot(i *interp.Interpreter) (returnCode int64) {
	//parse bot admin list
	var botAdmin []int
	botAdminStr := strings.Split(config["BotAdmin"], ",")
	if len(botAdminStr) == 0 && config["BotAdmin"] != "" {
		botAdminStr = []string{config["BotAdmin"]}
	}
	if len(botAdminStr) != 0 {
		for _, s := range botAdminStr {
			id, err := strconv.Atoi(s)
			if err == nil {
				botAdmin = append(botAdmin, id)
			}
		}
	}

	var botAPI = "https://api.telegram.org"
	if config["BotAPI"] != "" {
		botAPI = config["BotAPI"]
	}

	//set logger interface for bot
	err := tgbotapi.SetLogger(logrus.StandardLogger())
	if err != nil {
		logrus.Errorln(err)
		return 1
	}
	// set token、api、debug
	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint(config["BOT_TOKEN"], botAPI+"/bot%s/%s")
	if err != nil {
		logrus.Errorln(err)
		return 1
	}
	if config["BotDebug"] == "true" {
		bot.Debug = true
	}

	logrus.Printf("%s 验证成功", bot.Self.UserName)

	for _, f := range Register.OnStart {
		err := (*f)(bot, config, i)
		if err != nil {
			logrus.Errorln(err)
		}
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
	defer bot.StopReceivingUpdates()

	for update := range updates {
		if update.Message == nil && update.CallbackQuery == nil && update.InlineQuery == nil { // ignore any non-Message Updates
			continue
		}
		message := *update.Message
		switch {
		case update.Message != nil:
			if message.Command() != "" {
				if atStr := strings.ReplaceAll(message.CommandWithAt(), message.Command(), ""); atStr != "" && atStr != "@"+bot.Self.UserName {
					continue
				}
				switch message.Command() {
				case "reload":
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "重新加载中...")
					msg.ReplyToMessageID = update.Message.MessageID
					_, _ = bot.Send(msg)
					break
				default:
					f := Register.Commands[message.Command()]
					if f != nil {
						err := (*f)(bot, message)
						if err != nil {
							logrus.Errorln(err)
						}
					}
				}
			} else {
				switch message.Command() {
				case "help":
					//todo: help命令
				default:
					for _, f := range Register.OnReceiveMessage {
						err := (*f)(bot, message)
						if err != nil {
							logrus.Errorln(err)
						}
					}
				}
			}
		case update.CallbackQuery != nil:
			for _, f := range Register.OnReceiveCallbackQuery {
				err := (*f)(bot, message)
				if err != nil {
					logrus.Errorln(err)
				}
			}
		case update.InlineQuery != nil:
			for _, f := range Register.OnReceiveInlineQuery {
				err := (*f)(bot, message)
				if err != nil {
					logrus.Errorln(err)
				}
			}
		}
	}
	return 0
}

func parseCommands(module ModuleInfo) (list, help map[string]string) {
	list = make(map[string]string)
	help = make(map[string]string)
	for _, c := range module.RegisterFunc.Commands {
		if c.AddToList {
			list[c.Command] = c.Description
		}
		if c.AddToHelp {
			if c.HelpText != "" {
				help[c.Command] = c.HelpText
			} else {
				help[c.Command] = c.Description
			}
		}
	}
	return
}

func getModuleUpdate(module *Module) (err error) {
	if module.Type != "remote" {
		return
	}
	remoteInfo, err := getRemoteModuleInfo(*module)
	if err != nil {
		return err
	}
	module.Path = remoteInfo.ModulePath
	if !dirExists(fmt.Sprintf("%s/%s", *modulePath, module.Path)) {
		err = os.MkdirAll(fmt.Sprintf("%s/%s", *modulePath, module.Path), os.ModePerm)
		if err != nil {
			return err
		}
	}
	if !fileExists(fmt.Sprintf("%s/%s/%s", *modulePath, module.Path, path.Base(module.Url))) {
		err = getFileFromURL(module.Url, fmt.Sprintf("%s/%s/%s", *modulePath, module.Path, path.Base(module.Url)))
		if err != nil {
			return err
		}
		return downloadModuleFiles(remoteInfo)
	}
	//compare local version with remote version
	moduleInfo, err := getLocalModuleInfo(*module)
	if err != nil {
		return err
	}
	if moduleInfo.VersionCode > remoteInfo.VersionCode {
		err = getFileFromURL(module.Url, fmt.Sprintf("%s/%s/%s", *modulePath, module.Path, path.Base(module.Url)))
		if err != nil {
			return err
		}
		return downloadModuleFiles(remoteInfo)
	}
	return nil
}

func getLocalModuleInfo(module Module) (moduleInfo *ModuleInfo, err error) {
	if module.Path == "" {
		return nil, nil
	}
	moduleJsonPath := "/" + path.Base(module.Url)
	if module.Type == "local" {
		moduleJsonPath = ""
	}
	content, err := ioutil.ReadFile(fmt.Sprintf("%s/%s%s", *modulePath, module.Path, moduleJsonPath))
	if err != nil {
		return nil, err
	}
	moduleInfo = &ModuleInfo{}
	err = json.Unmarshal(content, moduleInfo)
	if err != nil {
		return nil, err
	}
	return moduleInfo, nil
}

func getRemoteModuleInfo(module Module) (moduleInfo ModuleInfo, err error) {
	resp, err := http.Get(module.Url)
	if err != nil {
		return moduleInfo, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	content, _ := ioutil.ReadAll(resp.Body)
	moduleInfo = ModuleInfo{}
	err = json.Unmarshal(content, &moduleInfo)
	if err != nil {
		return moduleInfo, err
	}
	return moduleInfo, nil
}

func downloadModuleFiles(module ModuleInfo) (err error) {
	for _, file := range module.Files {
		for _, url := range module.UpdateUrls {
			err = getFileFromURL(fmt.Sprintf("%s/%s", url, file.Name), fmt.Sprintf("%s/%s/%s", *modulePath, module.ModulePath, file.Name))
			if err != nil {
				logrus.Errorln(err)
				continue
			}
			break
		}
	}
	return nil
}

func newInterpreter() *interp.Interpreter {
	i := interp.New(interp.Options{})
	_ = i.Use(unsafe.Symbols)
	_ = i.Use(stdlib.Symbols)
	_ = i.Use(syscall.Symbols)
	_ = i.Use(unrestricted.Symbols)
	_ = i.Use(symbols.Symbols)
	return i
}

func registerFunc(i *interp.Interpreter, m ModuleInfo) (err error) {
	for _, s := range m.RegisterFunc.OnStart {
		res, err := evalFunc(i, fmt.Sprintf("%s.%s", m.Package, s))
		if err != nil {
			logrus.Println(err)
			continue
		}
		f, ok := res.Interface().(func(bot *tgbotapi.BotAPI, config map[string]string, i *interp.Interpreter) (err error))
		if ok {
			Register.OnStart = append(Register.OnStart, &f)
		}
	}
	for _, s := range m.RegisterFunc.OnStop {
		res, err := evalFunc(i, fmt.Sprintf("%s.%s", m.Package, s))
		if err != nil {
			logrus.Println(err)
			continue
		}
		f, ok := res.Interface().(func(bot *tgbotapi.BotAPI, config map[string]string, i *interp.Interpreter) (err error))
		if ok {
			Register.OnStop = append(Register.OnStop, &f)
		}
	}
	for _, s := range m.RegisterFunc.OnReceiveMessage {
		res, err := evalFunc(i, fmt.Sprintf("%s.%s", m.Package, s))
		if err != nil {
			logrus.Println(err)
			continue
		}
		f, ok := res.Interface().(func(bot *tgbotapi.BotAPI, message tgbotapi.Message) (err error))
		if ok {
			Register.OnReceiveMessage = append(Register.OnReceiveMessage, &f)
		}
	}
	for _, s := range m.RegisterFunc.OnReceiveInlineQuery {
		res, err := evalFunc(i, fmt.Sprintf("%s.%s", m.Package, s))
		if err != nil {
			logrus.Println(err)
			continue
		}
		f, ok := res.Interface().(func(bot *tgbotapi.BotAPI, message tgbotapi.Message) (err error))
		if ok {
			Register.OnReceiveInlineQuery = append(Register.OnReceiveInlineQuery, &f)
		}
	}
	for _, s := range m.RegisterFunc.OnReceiveEmptyInlineQuery {
		res, err := evalFunc(i, fmt.Sprintf("%s.%s", m.Package, s))
		if err != nil {
			logrus.Println(err)
			continue
		}
		f, ok := res.Interface().(func(bot *tgbotapi.BotAPI, message tgbotapi.Message) (err error))
		if ok {
			Register.OnReceiveEmptyInlineQuery = append(Register.OnReceiveEmptyInlineQuery, &f)
		}
	}
	for _, s := range m.RegisterFunc.OnReceiveCallbackQuery {
		res, err := evalFunc(i, fmt.Sprintf("%s.%s", m.Package, s))
		if err != nil {
			logrus.Println(err)
			continue
		}
		f, ok := res.Interface().(func(bot *tgbotapi.BotAPI, message tgbotapi.Message) (err error))
		if ok {
			Register.OnReceiveCallbackQuery = append(Register.OnReceiveCallbackQuery, &f)
		}
	}
	for _, s := range m.RegisterFunc.Commands {
		res, err := evalFunc(i, fmt.Sprintf("%s.%s", m.Package, s.Func))
		if err != nil {
			logrus.Println(err)
			continue
		}
		f, ok := res.Interface().(func(bot *tgbotapi.BotAPI, message tgbotapi.Message) (err error))
		if ok {
			Register.Commands[s.Command] = &f
		}
	}
	return nil
}

func evalFunc(i *interp.Interpreter, s string) (_ *reflect.Value, err error) {
	defer func() {
		e := recover()
		if e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()
	res, err := i.Eval(s)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func loadModule(i *interp.Interpreter, m ModuleInfo) error {
	for _, file := range m.Files {
		_, err := i.EvalPath(fmt.Sprintf("%s/%s/%s", *modulePath, m.ModulePath, file.Name))
		if err != nil {
			logrus.Errorln(err)
			continue
		}
	}
	return nil
}

func readConfig(path string) (config map[string]string, err error) {
	config = make(map[string]string)
	f, err := os.Open(path)
	if err != nil {
		return config, err
	}
	defer func(f *os.File) {
		e := f.Close()
		if e != nil {
			err = e
		}
	}(f)
	r := bufio.NewReader(f)
	for {
		b, _, err := r.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return config, err
		}
		s := strings.TrimSpace(string(b))
		index := strings.Index(s, "=")
		if index < 0 {
			continue
		}
		key := strings.TrimSpace(s[:index])
		if len(key) == 0 {
			continue
		}
		value := strings.TrimSpace(s[index+1:])
		if len(value) == 0 {
			continue
		}
		config[key] = value
	}
	return config, err
}

func fileExists(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

func dirExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		err := os.Mkdir(path, os.ModePerm)
		if err != nil {
			logrus.Errorf("mkdir %v failed: %v\n", path, err)
		}
		return false
	}
	logrus.Errorf("Error: %v\n", err)
	return false
}

func getFileFromURL(url string, savePath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	file, err := os.Create(fmt.Sprintf(savePath))
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}
	return nil
}

func initConfig(config map[string]string) {
	config["BinVersionName"] = versionName
	config["BinVersionCode"] = versionCode
	config["RuntimeVer"] = runtimeVer
	config["BuildTime"] = buildTime
	config["CommitSHA"] = commitSHA
	config["BuildArch"] = buildArch
	config["RepoPath"] = repoPath
}
