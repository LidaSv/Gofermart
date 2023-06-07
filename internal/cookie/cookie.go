package cookie

import (
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/url"
	"time"
)

func SetCookie(login string) http.Cookie {
	hashToken := md5.Sum([]byte(login))
	hashedToken := hex.EncodeToString(hashToken[:])
	livingTime := 60 * time.Minute
	expiration := time.Now().Add(livingTime)
	cookie := http.Cookie{Name: "token", Value: url.QueryEscape(hashedToken), Expires: expiration}
	return cookie
}
