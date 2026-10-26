package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

const jwtHeader = `{"alg":"HS256","typ":"JWT"}`

type tokenClaims struct {
	Subject string `json:"sub"`
	Expires int64  `json:"exp"`
	Issued  int64  `json:"iat"`
}

func (a *app) signature(message string) string {
	mac := hmac.New(sha256.New, a.jwtSecret)
	mac.Write([]byte(message))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *app) issueToken(userID int64) (string, error) {
	now := time.Now().Unix()
	claims, err := json.Marshal(tokenClaims{Subject: strconv.FormatInt(userID, 10), Expires: now + int64(a.tokenTTL.Seconds()), Issued: now})
	if err != nil {
		return "", err
	}
	message := base64.RawURLEncoding.EncodeToString([]byte(jwtHeader)) + "." + base64.RawURLEncoding.EncodeToString(claims)
	return message + "." + a.signature(message), nil
}

func (a *app) verifyToken(header string) (int64, int64, string, bool) {
	parts := strings.Split(header, " ")
	if len(parts) != 2 || parts[0] != "Bearer" || len(parts[1]) > 4096 {
		return 0, 0, "", false
	}
	token := parts[1]
	segments := strings.Split(token, ".")
	if len(segments) != 3 {
		return 0, 0, "", false
	}
	h, err := base64.RawURLEncoding.DecodeString(segments[0])
	if err != nil || string(h) != jwtHeader {
		return 0, 0, "", false
	}
	want, err := base64.RawURLEncoding.DecodeString(segments[2])
	if err != nil {
		return 0, 0, "", false
	}
	mac := hmac.New(sha256.New, a.jwtSecret)
	mac.Write([]byte(segments[0] + "." + segments[1]))
	if !hmac.Equal(want, mac.Sum(nil)) {
		return 0, 0, "", false
	}
	data, err := base64.RawURLEncoding.DecodeString(segments[1])
	if err != nil {
		return 0, 0, "", false
	}
	var claims tokenClaims
	if json.Unmarshal(data, &claims) != nil {
		return 0, 0, "", false
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	now := time.Now().Unix()
	if err != nil || id < 1 || claims.Issued > now || claims.Expires <= now || claims.Expires <= claims.Issued {
		return 0, 0, "", false
	}
	return id, claims.Expires, token, true
}
