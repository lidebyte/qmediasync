package baidupan

import (
	"Q115-STRM/internal/helpers"
	"context"
	"fmt"
)

const AUTH_SERVER_URL = "https://api.mqfamily.top/baidupan"

// GetUserInfo 获取用户信息
func (c *BaiDuPanClient) GetUserInfo(ctx context.Context) (*UserInfoResponse, error) {
	url := fmt.Sprintf("%s/user/info", AUTH_SERVER_URL)
	req := c.client.R()

	var userInfo UserInfoResponse
	_, _, err := c.doAuthRequest(ctx, url, req, DefaultRequestConfig(), &userInfo)
	if err != nil {
		helpers.AppLogger.Errorf("获取用户信息失败: %v", err)
		return nil, fmt.Errorf("获取用户信息失败: %v", err)
	}

	helpers.AppLogger.Infof("成功获取用户信息: userId=%s, username=%s", userInfo.UserId, userInfo.Username)
	return &userInfo, nil
}

// RefreshToken 刷新访问令牌
func (c *BaiDuPanClient) RefreshToken(ctx context.Context) error {
	url := fmt.Sprintf("%s/oauth/refresh", AUTH_SERVER_URL)

	type refreshTokenReq struct {
		RefreshToken string `json:"refresh_token"`
	}

	reqBody := refreshTokenReq{
		RefreshToken: c.RefreshTokenStr,
	}

	req := c.client.R().SetBody(reqBody)

	type refreshTokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}

	var resp refreshTokenResp
	_, _, err := c.doRequest(url, req, DefaultRequestConfig())
	if err != nil {
		helpers.AppLogger.Errorf("刷新令牌失败: %v", err)
		return fmt.Errorf("刷新令牌失败: %v", err)
	}

	if resp.AccessToken != "" {
		c.AccessToken = resp.AccessToken
		c.RefreshTokenStr = resp.RefreshToken
		helpers.AppLogger.Infof("成功刷新访问令牌")
	}

	return nil
}
