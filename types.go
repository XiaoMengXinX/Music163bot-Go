package main

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/traefik/yaegi/interp"
)

type Module struct {
	Type    string `json:"type"`
	Url     string `json:"url,omitempty"`
	Enabled bool   `json:"enabled"`
	Path    string `json:"path,omitempty"`
}

type Modules struct {
	Modules []Module `json:"modules"`
}

type ModuleInfo struct {
	Name        string   `json:"name"`
	Package     string   `json:"package"`
	ModulePath  string   `json:"module_path"`
	Version     string   `json:"version"`
	VersionCode int      `json:"version_code"`
	HelpText    string   `json:"help_text"`
	UpdateUrls  []string `json:"update_urls"`
	Files       []struct {
		Name string `json:"name"`
	} `json:"files"`
	RegisterFunc struct {
		OnStart                   []string `json:"OnStart"`
		OnStop                    []string `json:"OnStop"`
		OnReceiveMessage          []string `json:"OnReceiveMessage"`
		OnReceiveInlineQuery      []string `json:"OnReceiveInlineQuery"`
		OnReceiveEmptyInlineQuery []string `json:"OnReceiveEmptyInlineQuery"`
		OnReceiveCallbackQuery    []string `json:"OnReceiveCallbackQuery"`
		Commands                  []struct {
			Func        string `json:"func"`
			Command     string `json:"command"`
			Description string `json:"description"`
			HelpText    string `json:"help_text"`
			AddToHelp   bool   `json:"add_to_help"`
			AddToList   bool   `json:"add_to_list"`
		} `json:"Commands"`
	} `json:"register_func"`
}

type FuncCommands map[string]*func(bot *tgbotapi.BotAPI, message tgbotapi.Message) (err error)

type FuncRegister struct {
	OnStart                   []*func(bot *tgbotapi.BotAPI, config map[string]string, i *interp.Interpreter) (err error)
	OnStop                    []*func(bot *tgbotapi.BotAPI, config map[string]string, i *interp.Interpreter) (err error)
	OnReceiveMessage          []*func(bot *tgbotapi.BotAPI, message tgbotapi.Message) (err error)
	OnReceiveInlineQuery      []*func(bot *tgbotapi.BotAPI, message tgbotapi.Message) (err error)
	OnReceiveEmptyInlineQuery []*func(bot *tgbotapi.BotAPI, message tgbotapi.Message) (err error)
	OnReceiveCallbackQuery    []*func(bot *tgbotapi.BotAPI, message tgbotapi.Message) (err error)
	Commands                  FuncCommands
}
