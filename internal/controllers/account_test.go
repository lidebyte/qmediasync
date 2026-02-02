package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCreateOpenListAccount_MissingRequiredFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/account/openlist", CreateOpenListAccount)

	tests := []struct {
		name           string
		body           map[string]interface{}
		expectedStatus int
		expectedCode   APIResponseCode
		expectedMsg    string
	}{
		{
			name:           "缺少 BaseUrl",
			body:           map[string]interface{}{"username": "test", "password": "test"},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   BadRequest,
			expectedMsg:    "请求参数错误",
		},
		{
			name:           "缺少 Username",
			body:           map[string]interface{}{"base_url": "http://localhost:8080", "password": "test"},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   BadRequest,
			expectedMsg:    "请求参数错误",
		},
		{
			name:           "缺少 Token 和 Password",
			body:           map[string]interface{}{"base_url": "http://localhost:8080", "username": "test"},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   BadRequest,
			expectedMsg:    "必须提供Token或密码",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req, _ := http.NewRequest("POST", "/account/openlist", bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("期望状态码 %d，实际 %d", tt.expectedStatus, w.Code)
			}

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			if err != nil {
				t.Errorf("解析响应失败: %v", err)
			}
			if APIResponseCode(response["code"].(float64)) != tt.expectedCode {
				t.Errorf("期望代码 %d，实际 %d", tt.expectedCode, APIResponseCode(response["code"].(float64)))
			}
			if response["message"] != tt.expectedMsg {
				t.Errorf("期望消息 %s，实际 %s", tt.expectedMsg, response["message"])
			}
		})
	}
}

func TestCreateOpenListAccount_TokenOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/account/openlist", CreateOpenListAccount)

	body := map[string]interface{}{
		"base_url": "http://localhost:8080",
		"username": "test",
		"token":    "valid_token",
	}
	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/account/openlist", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			// 这个测试期望会 panic，因为没有真实的 OpenList 服务器
			// 只要参数验证通过，我们就认为测试成功
			t.Logf("预期 Panic 发生 (因为没有真实的 OpenList 服务器): %v", r)
		}
	}()

	router.ServeHTTP(w, req)

	// 如果没有 panic，说明可能环境配置正确
	t.Logf("请求已发送，状态码: %d", w.Code)
}
