package ths

// 账密登录:verify2 三连(取公钥 → unified_login → mainverify)→ docookie2 换 cookie。
// 协议事实经离线探针实测确认(见 cmd/probe),**纯 stdlib 实现**:
// crypto/rsa + crypto/x509(不引 cryptography 等价物)、encoding/xml、net/url。
//
// 🔴 无滑块、无设备指纹 —— 比网页版登录简单。这是「真全自动」可行的前提。
//
// 所有解析都做成**纯函数**(parseRet/parseItemAttrs/parsePassport/jwtExpiry/…),便于离线
// fixture 单测(协议一改,单测先红,而不是线上静默失败 —— #64 教训)。

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	authBase  = "https://auth.10jqka.com.cn"
	upassBase = "https://upass.10jqka.com.cn"

	// 抓包得到的移动端常量(参照实现 config/auth.py 实测值,不臆造)。
	qsid         = "8003"
	product      = "S01"
	versionParam = "11.4.1.3"
	imeiEncoded  = "ZjI6MDY6NGE6NzI6MjQ6NTA="
	// URL 编码后的「同花顺远航版」(securities 参数)。
	securities       = "%E5%90%8C%E8%8A%B1%E9%A1%BA%E8%BF%9C%E8%88%AA%E7%89%88"
	rsaVersionFallbk = "default_5"
	taAppID          = "2022021114090152"
)

var errItemMissing = errors.New("ths: 响应缺少 <item> 节点")

// Login 账密登录并装载会话。失败返回带上游 msg 的错误(不美化)。
func (c *Client) Login(ctx context.Context, account, password string) error {
	pub, rsaVer, err := c.fetchRSAPubkey(ctx)
	if err != nil {
		return fmt.Errorf("取公钥: %w", err)
	}
	encAccount, err := encryptPKCS1v15(pub, account)
	if err != nil {
		return fmt.Errorf("加密账号: %w", err)
	}
	encPassword, err := encryptPKCS1v15(pub, password)
	if err != nil {
		return fmt.Errorf("加密密码: %w", err)
	}
	userid, sessionid, err := c.unifiedLogin(ctx, encAccount, encPassword, rsaVer)
	if err != nil {
		return fmt.Errorf("登录: %w", err)
	}
	signvalid, err := c.mainVerify(ctx, userid, sessionid, rsaVer)
	if err != nil {
		return fmt.Errorf("主验证: %w", err)
	}
	cookies, err := c.docookie(ctx, userid, sessionid, signvalid)
	if err != nil {
		return fmt.Errorf("换 cookie: %w", err)
	}
	c.setSession(cookies, userid)
	return nil
}

// fetchRSAPubkey ① GET verify2?reqtype=do_rsa&type=get_pubkey。
func (c *Client) fetchRSAPubkey(ctx context.Context) (string, string, error) {
	u := withQuery(authBase+"/verify2", "reqtype", "do_rsa", "type", "get_pubkey")
	body, err := c.getText(ctx, u, nil)
	if err != nil {
		return "", "", err
	}
	attrs, err := parseItemAttrs(body)
	if err != nil {
		return "", "", err
	}
	pub := attrs["pubkey"]
	if pub == "" {
		return "", "", errors.New("缺少 pubkey 属性")
	}
	ver := attrs["rsa_version"]
	if ver == "" {
		ver = rsaVersionFallbk
	}
	return pub, ver, nil
}

// unifiedLogin ② GET verify2?reqtype=unified_login&account=&passwd=&…
func (c *Client) unifiedLogin(ctx context.Context, encAccount, encPassword, rsaVer string) (string, string, error) {
	u := withQuery(authBase+"/verify2",
		"account", encAccount,
		"msg", "1",
		"passwd", encPassword,
		"reqtype", "unified_login",
		"rsa_version", rsaVer,
		"ta_appid", taAppID,
	)
	body, err := c.getText(ctx, u, nil)
	if err != nil {
		return "", "", err
	}
	attrs, err := parseItemAttrs(body)
	if err != nil {
		return "", "", err
	}
	userid, sessionid, account := attrs["userid"], attrs["sessionid"], attrs["account"]
	if userid == "" || sessionid == "" || account == "" {
		return "", "", fmt.Errorf("缺少 userid/sessionid/account(userid=%q)", userid)
	}
	return userid, sessionid, nil
}

// mainVerify ③ GET verify2?reqtype=mainverify&… → 取 passport 里的 signvalid。
func (c *Client) mainVerify(ctx context.Context, userid, sessionid, rsaVer string) (string, error) {
	u := withQuery(authBase+"/verify2",
		"reqtype", "mainverify",
		"userid", userid,
		"sessionid", sessionid,
		"qsid", qsid,
		"product", product,
		"version", versionParam,
		"imei", imeiEncoded,
		"sdsn", "",
		"rsa_version", rsaVer,
		"nohqlist", "0",
		"securities", securities,
	)
	body, err := c.getText(ctx, u, nil)
	if err != nil {
		return "", err
	}
	attrs, err := parseItemAttrs(body)
	if err != nil {
		return "", err
	}
	blob := attrs["passport"]
	if blob == "" {
		return "", errors.New("缺少 passport 数据")
	}
	signvalid := parsePassport(blob)["signvalid"]
	if signvalid == "" {
		return "", errors.New("passport 内无 signvalid")
	}
	return signvalid, nil
}

// docookie ④ GET docookie2.php?userid=&sessionid=&signvalid= → Set-Cookie。
// ⚠️ 该端点的 Set-Cookie 可能是**逗号分隔多 cookie**,须用 parseSetCookieHeader 解析。
func (c *Client) docookie(ctx context.Context, userid, sessionid, signvalid string) (map[string]string, error) {
	u := withQuery(upassBase+"/docookie2.php",
		"userid", userid, "sessionid", sessionid, "signvalid", signvalid)
	body, err := c.doRawCookieFetch(ctx, u)
	if err != nil {
		return nil, err
	}
	cookies := parseSetCookieHeader(body)
	if len(cookies) == 0 {
		return nil, errors.New("docookie2.php 未返回任何 cookie")
	}
	return cookies, nil
}

// doRawCookieFetch 直接取 Set-Cookie 头(不能走 once —— 那里不暴露 headers)。
func (c *Client) doRawCookieFetch(ctx context.Context, rawURL string) (string, error) {
	req, err := c.newGetReq(ctx, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	// 上游可能直接在 Set-Cookie 回多枚;全部拼起来再解析。
	var parts []string
	for _, sc := range resp.Header.Values("Set-Cookie") {
		parts = append(parts, sc)
	}
	return strings.Join(parts, ", "), nil
}

// ── 纯解析函数(fixture 可单测,零网络)─────────────────────────────────────

// parseRet 解析响应里的 <ret code msg>,校验 code==0。缺 <ret> 或非零 → 错误(不空成功)。
// ⚠️ <ret> **不一定是根节点** —— selfstock_detail 的外层是 `<download><ret …/><item …/></download>`
// (2026-09-22 实测)。Go 的 encoding/xml 无法把「根元素」映射到具名切片字段,故用
// token 扫描找**首个** <ret>(根或后代一视同仁)。
func parseRet(body []byte) (code, msg string, err error) {
	se, err := firstStartElement(body, "ret")
	if err != nil {
		return "", "", err
	}
	for _, a := range se.Attr {
		switch a.Name.Local {
		case "code":
			code = a.Value
		case "msg":
			msg = a.Value
		}
	}
	if code != "0" {
		if msg == "" {
			msg = "未知错误"
		}
		return code, msg, fmt.Errorf("上游 code=%s msg=%s", code, msg)
	}
	return code, msg, nil
}

// parseItemAttrs 解析响应里首个 <item k=v …/> 的属性表。
// 校验 <ret code=0>;缺 <item> → 错误。
func parseItemAttrs(body []byte) (map[string]string, error) {
	if _, _, err := parseRet(body); err != nil {
		return nil, err
	}
	se, err := firstStartElement(body, "item")
	if err != nil {
		if errors.Is(err, errItemMissing) {
			return nil, errItemMissing
		}
		return nil, err
	}
	m := make(map[string]string, len(se.Attr))
	for _, a := range se.Attr {
		m[a.Name.Local] = a.Value
	}
	return m, nil
}

// firstStartElement 用流式解码找首个指定名的开始元素(根或任意深度后代)。
// 找不到 → errItemMissing(调用方按需包装);XML 非法 → 解析错误。
func firstStartElement(body []byte, name string) (xml.StartElement, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return xml.StartElement{}, errItemMissing
		}
		if err != nil {
			return xml.StartElement{}, fmt.Errorf("XML 解析失败: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == name {
			return se, nil
		}
	}
}

// parsePassport 解析 passport blob("a=1|b=2|…") → map。
func parsePassport(blob string) map[string]string {
	out := map[string]string{}
	for _, chunk := range strings.Split(blob, "|") {
		k, v, ok := strings.Cut(chunk, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

// parseCookieString 解析用户注入的整串 Cookie 头("a=1; b=2")。
func parseCookieString(raw string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(raw, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

// parseSetCookieHeader 解析 Set-Cookie 风格头(可能**逗号分隔多枚**)。
// ⚠️ 每枚只取第一个 "key=value"(分号后是 Path/Domain 等属性,丢弃)。
func parseSetCookieHeader(header string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(header, ",") {
		seg := strings.TrimSpace(part)
		if seg == "" {
			continue
		}
		pair, _, _ := strings.Cut(seg, ";")
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = strings.TrimSpace(v)
	}
	return out
}

// jwtExpiry 解 JWT payload 的 exp(**不验签** —— 只为本地判断续期时机)。
// 非 JWT / 无 exp / 解析失败 → (零值, false)。
func jwtExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// 容错:部分实现带 padding。
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return time.Time{}, false
		}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}

// encryptPKCS1v15 用 PEM 公钥对明文做 RSA PKCS#1 v1.5 加密 → base64(PEM 可含头尾)。
// 纯 stdlib(crypto/rsa + crypto/x509 + encoding/pem)。
func encryptPKCS1v15(pubPEM, plain string) (string, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pubPEM)))
	if block == nil {
		return "", errors.New("公钥不是合法 PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		// 兼容 PKCS#1 公钥形态。
		pk1, e2 := x509.ParsePKCS1PublicKey(block.Bytes)
		if e2 != nil {
			return "", fmt.Errorf("解析公钥: %w", err)
		}
		pub = pk1
	}
	rpub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return "", errors.New("公钥不是 RSA")
	}
	enc, err := rsa.EncryptPKCS1v15(rand.Reader, rpub, []byte(plain))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(enc), nil
}
