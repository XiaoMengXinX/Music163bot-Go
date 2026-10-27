package bot

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestExpectedErrorHook(t *testing.T) {
	for _, tt := range []struct {
		msg  string
		want string
	}{
		{msg: "bad request, Bad Request: not enough rights to send text messages to the chat", want: "level=warning"},
		{msg: "bad request, Bad Request: TOPIC_CLOSED", want: "level=warning"},
		{msg: "bad request, Bad Request: message to edit not found", want: "level=warning"},
		{msg: "bad request, Bad Request: message to delete not found", want: "level=warning"},
		{msg: "too many requests, Too Many Requests: retry after 8: retry_after 8", want: "level=warning"},
		{msg: "bad request, Bad Request: can't parse entities", want: "level=error"},
	} {
		var buf bytes.Buffer
		log := logrus.New()
		log.SetOutput(&buf)
		log.AddHook(expectedErrorHook{})
		log.Errorln(tt.msg)
		if !strings.Contains(buf.String(), tt.want) {
			t.Errorf("%q logged as %q, want %s", tt.msg, buf.String(), tt.want)
		}
	}
}
