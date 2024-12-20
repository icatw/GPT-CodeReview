package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Config 配置文件结构体
type Config struct {
	APIKey      string `json:"api_key"`
	ModelName   string `json:"model_name"`
	BaseURL     string `json:"base_url"`
	DingWebhook string `json:"ding_webhook"`
	DingSecret  string `json:"ding_secret"`
}

// 全局配置
var config Config

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type RequestBody struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type ResponseBody struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// 加载配置文件
func loadConfig(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}
	return nil
}

// Generate HMAC-SHA256 签名
func generateSign(secret string, timestamp int64) (string, error) {
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
	h := hmac.New(sha256.New, []byte(secret))
	_, err := h.Write([]byte(stringToSign))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
}

// 发送钉钉消息
func sendDingMessage(message string) error {
	timestamp := time.Now().UnixMilli()
	sign, err := generateSign(config.DingSecret, timestamp)
	if err != nil {
		return fmt.Errorf("failed to generate DingTalk sign: %w", err)
	}

	// 构造完整的 Webhook URL
	webhookURL := fmt.Sprintf("%s&timestamp=%d&sign=%s", config.DingWebhook, timestamp, url.QueryEscape(sign))

	// 构造消息内容
	body := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": message,
		},
	}
	jsonData, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal DingTalk message body: %w", err)
	}

	// 发送 HTTP POST 请求
	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send DingTalk message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to send message, status code: %d", resp.StatusCode)
	}

	log.Println("Message sent successfully!")
	return nil
}

// 代码评审逻辑
func performCodeReview(diffContent string) (string, error) {
	payload := RequestBody{
		Model: config.ModelName,
		Messages: []Message{
			{
				Role:    "system",
				Content: "你是一个高级编程架构师，请根据以下 git diff 内容提供代码评审建议：",
			},
			{
				Role:    "user",
				Content: diffContent,
			},
		},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	// 发送审查请求
	req, err := http.NewRequest("POST", config.BaseURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request failed with status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result ResponseBody
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response JSON: %w", err)
	}

	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}
	return "No review results returned.", nil
}

func main() {
	// 加载配置文件
	err := loadConfig("conf/config.json")
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	// 读取标准输入中的代码差异内容
	var diffContent strings.Builder
	_, err = io.Copy(&diffContent, os.Stdin)
	if err != nil {
		log.Fatalf("Failed to read diff content: %v", err)
	}
	// 执行代码评审
	reviewResult, err := performCodeReview(diffContent.String())
	if err != nil {
		log.Printf("Code review failed: %v", err)
		fmt.Print("No valid review result.\n") // 明确的错误输出
		return
	}

	// 输出评审结果（仅输出结果内容）
	fmt.Println(reviewResult)

	// 发送钉钉消息
	if err := sendDingMessage(reviewResult); err != nil {
		log.Printf("Failed to send DingTalk message: %v", err)
	}

	log.Println("All tasks completed.")
}
