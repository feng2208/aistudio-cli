package api

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Expose-Headers", "*")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loopbackHost 判断 Host 或 Origin 主机名是否为 localhost 或回环地址
func loopbackHost(host string) bool {
	name := host
	if hostname, _, err := net.SplitHostPort(host); err == nil {
		name = hostname
	}
	name = strings.Trim(name, "[]")
	if strings.EqualFold(name, "localhost") {
		return true
	}
	ip := net.ParseIP(name)
	return ip != nil && ip.IsLoopback()
}

// browserOriginMiddleware 在未配置 API key 时拒绝外部网页与 null 来源的浏览器请求
func browserOriginMiddleware(requiredKey string, next http.Handler) http.Handler {
	if strings.TrimSpace(requiredKey) != "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originValue := strings.TrimSpace(r.Header.Get("Origin"))
		if originValue == "" {
			next.ServeHTTP(w, r)
			return
		}
		origin, err := url.Parse(originValue)
		webOrigin := err != nil || originValue == "null" || origin.Scheme == "http" || origin.Scheme == "https"
		if webOrigin && (err != nil || !loopbackHost(origin.Host)) {
			writeAuthError(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// maxPublicBodyBytes 为公开接口请求体上限，可容纳 Base64 编码后的最大文件
const maxPublicBodyBytes = openAIFileMaxBytes/3*4 + openAIFileRequestOverhead

// bodyLimitMiddleware 限制公开接口请求体大小
func bodyLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxPublicBodyBytes)
		next.ServeHTTP(w, r)
	})
}

func authMiddleware(configuredKeys string, next http.Handler) http.Handler {
	var keys []string
	for key := range strings.SplitSeq(configuredKeys, ",") {
		if key = strings.TrimSpace(key); key != "" {
			keys = append(keys, key)
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(keys) > 0 && !checkAPIKeyMatches(r, keys) {
			writeAuthError(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// apiKeyCandidates 返回请求头与 URL 中给出的 API key，去掉首尾空白
func apiKeyCandidates(r *http.Request) []string {
	candidates := []string{
		r.Header.Get("X-Goog-API-Key"),
		r.Header.Get("X-API-Key"),
		bearerToken(r.Header.Get("Authorization")),
		r.URL.Query().Get("key"),
	}
	for index, candidate := range candidates {
		candidates[index] = strings.TrimSpace(candidate)
	}
	return candidates
}

// checkAPIKeyMatches 判断请求头或 URL 中任一 API key 是否与配置的某个 key 相同
func checkAPIKeyMatches(r *http.Request, keys []string) bool {
	for _, cand := range apiKeyCandidates(r) {
		if cand == "" {
			continue
		}
		for _, key := range keys {
			if subtle.ConstantTimeCompare([]byte(cand), []byte(key)) == 1 {
				return true
			}
		}
	}
	return false
}

// bearerToken 返回 Authorization Bearer 凭据
func bearerToken(authorization string) string {
	scheme, key, ok := strings.Cut(strings.TrimSpace(authorization), " ")
	if ok && strings.EqualFold(scheme, "Bearer") {
		return strings.TrimSpace(key)
	}
	return ""
}
