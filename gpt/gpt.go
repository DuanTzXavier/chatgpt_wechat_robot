package gpt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/qingconglaixueit/wechatbot/config"
)

// ChatGPTResponseBody 请求体
type ChatGPTResponseBody struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int                    `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChoiceItem           `json:"choices"`
	Usage   map[string]interface{} `json:"usage"`
	Error   struct {
		Message string      `json:"message"`
		Type    string      `json:"type"`
		Param   interface{} `json:"param"`
		Code    interface{} `json:"code"`
	} `json:"error"`
}

type ChoiceItem struct {
	Text         string `json:"text"`
	Index        int    `json:"index"`
	Logprobs     int    `json:"logprobs"`
	FinishReason string `json:"finish_reason"`
	Message      Msg    `json:"message"`
}

type Msg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatGPTRequestBody 响应体
type ChatGPTRequestBody struct {
	Model            string  `json:"model"`
	Messages         []Msg   `json:"messages,omitempty"`
	Prompt           string  `json:"prompt,omitempty"`
	MaxTokens        uint    `json:"max_tokens"`
	Temperature      float64 `json:"temperature"`
	TopP             int     `json:"top_p"`
	FrequencyPenalty int     `json:"frequency_penalty"`
	PresencePenalty  int     `json:"presence_penalty"`
}

// Completions gtp文本模型回复
// curl https://api.openai.com/v1/completions
// -H "Content-Type: application/json"
// -H "Authorization: Bearer your chatGPT key"
// -d '{"model": "text-davinci-003", "prompt": "give me good song", "temperature": 0, "max_tokens": 7}'
func Completions(uid string, msg string, fromAssistant bool) (string, string, error) {
	var gptResponseBody *ChatGPTResponseBody
	var resErr error
	var request = getRequest(uid, msg, fromAssistant)
	for retry := 1; retry <= 3; retry++ {
		if retry > 1 {
			time.Sleep(time.Duration(retry-1) * 100 * time.Millisecond)
		}
		gptResponseBody, resErr = httpRequestCompletions(request, retry)
		if resErr != nil {
			log.Printf("gpt request(%d) error: %v\n", retry, resErr)
			continue
		}
		if gptResponseBody.Error.Message == "" {
			break
		}
	}
	if resErr != nil {
		return "", "", resErr
	}
	var reply string
	var reason string = ""

	cfg := config.LoadConfig()
	if cfg.ApiKey == "" {
		return "", "", errors.New("api key required")
	}
	if cfg.Model == "gpt-3.5-turbo-0301" {
		if gptResponseBody != nil && len(gptResponseBody.Choices) > 0 {
			reply = gptResponseBody.Choices[0].Message.Content
			reason = gptResponseBody.Choices[0].FinishReason
		}
	} else {
		if gptResponseBody != nil && len(gptResponseBody.Choices) > 0 {
			reply = gptResponseBody.Choices[0].Text
			reason = gptResponseBody.Choices[0].FinishReason
		}
	}
	return reply, reason, nil
}

func getUserMsgArray(uid string, msg string) []Msg {
	addMsg(uid, Msg{
		Role:    "user",
		Content: msg,
	})
	msgs, _ := msgMap[uid]
	msgsJson, _ := json.Marshal(msgs)
	log.Printf("gpt %s request(%s)", msg, string(msgsJson))
	return msgs
}

func getAssistanMsgArray(uid string, msg string) []Msg {
	msgs, _ := msgMap[uid]
	return append(msgs, Msg{Role: "assistant",
		Content: msg})
}

// 假设需要存储的uid是int类型
var msgMap map[string][]Msg = make(map[string][]Msg)

// 存储Msg到uid对应的数组中
func addMsg(uid string, msg Msg) {
	msgs, ok := msgMap[uid]
	if !ok {
		// 如果uid对应的数组不存在，则创建一个新的数组
		msgs = make([]Msg, 0, 20)
	}
	// 将msg添加到数组中
	msgs = append(msgs, msg)
	// 如果数组的长度超过20，则删除最后一个元素
	if len(msgs) > 20 {
		msgs = msgs[1:]
	}
	// 更新msgMap中对应uid的数组
	msgMap[uid] = msgs
}

func getRequest(uid string, msg string, fromAssistant bool) ChatGPTRequestBody {
	cfg := config.LoadConfig()

	var requestBody ChatGPTRequestBody
	if cfg.Model == "gpt-3.5-turbo-0301" {
		var msgs []Msg
		if fromAssistant {
			msgs = getAssistanMsgArray(uid, msg)
		} else {
			msgs = getUserMsgArray(uid, msg)
		}

		requestBody = ChatGPTRequestBody{
			Model:            cfg.Model,
			Messages:         msgs,
			MaxTokens:        cfg.MaxTokens,
			Temperature:      cfg.Temperature,
			TopP:             1,
			FrequencyPenalty: 0,
			PresencePenalty:  0,
		}
	} else {
		requestBody = ChatGPTRequestBody{
			Model:            cfg.Model,
			Prompt:           msg,
			MaxTokens:        cfg.MaxTokens,
			Temperature:      cfg.Temperature,
			TopP:             1,
			FrequencyPenalty: 0,
			PresencePenalty:  0,
		}
	}
	return requestBody
}

func httpRequestCompletions(requestBody ChatGPTRequestBody, runtimes int) (*ChatGPTResponseBody, error) {
	cfg := config.LoadConfig()
	if cfg.ApiKey == "" {
		return nil, errors.New("api key required")
	}

	requestData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal requestBody error: %v", err)
	}

	log.Printf("gpt request(%d) json: %s\n", runtimes, string(requestData))

	var url string
	if cfg.Model == "gpt-3.5-turbo-0301" {
		url = "https://api.openai.com/v1/chat/completions"
	} else {
		url = "https://api.openai.com/v1/completions"
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(requestData))
	if err != nil {
		return nil, fmt.Errorf("http.NewRequest error: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.ApiKey)
	client := &http.Client{Timeout: 300 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("client.Do error: %v", err)
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("ioutil.ReadAll error: %v", err)
	}

	log.Printf("gpt response(%d) json: %s\n", runtimes, string(body))

	gptResponseBody := &ChatGPTResponseBody{}
	err = json.Unmarshal(body, gptResponseBody)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal responseBody error: %v", err)
	}
	return gptResponseBody, nil
}
