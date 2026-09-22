package ths

// 纯函数表驱动单测(零网络)。协议一改,这里先红 —— 而非线上静默失败(#64 教训)。

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestParseRet(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"成功", `<ret code="0" msg=""></ret>`, false},
		{"成功-包在 download 外层", `<?xml version="1.0"?><download>` + "\n" + `<ret code="0" msg=""/><item selfstock_detail="x"/></download>`, false},
		{"缺 ret 节点", `<foo code="0"></foo>`, true},
		{"非零 code", `<ret code="1" msg="登录失败"></ret>`, true},
		{"非 XML", `not xml`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := parseRet([]byte(c.body))
			if (err != nil) != c.wantErr {
				t.Fatalf("parseRet(%q) err=%v, wantErr=%v", c.body, err, c.wantErr)
			}
		})
	}
	// 非零 code 的错误信息须带上游 msg(不美化、不吞)。
	_, _, err := parseRet([]byte(`<ret code="1" msg="账号或密码错误"></ret>`))
	if err == nil || !contains(err.Error(), "账号或密码错误") {
		t.Fatalf("错误信息未带上游 msg: %v", err)
	}
}

func TestParseItemAttrs(t *testing.T) {
	body := []byte(`<?xml version="1.0"?><ret code="0"><item pubkey="-----BEGIN PUBLIC KEY-----" rsa_version="default_5"/></ret>`)
	attrs, err := parseItemAttrs(body)
	if err != nil {
		t.Fatal(err)
	}
	if attrs["pubkey"] != "-----BEGIN PUBLIC KEY-----" {
		t.Fatalf("pubkey=%q", attrs["pubkey"])
	}
	if attrs["rsa_version"] != "default_5" {
		t.Fatalf("rsa_version=%q", attrs["rsa_version"])
	}

	// 缺 <item> → 错误(不空成功)
	if _, err := parseItemAttrs([]byte(`<ret code="0"></ret>`)); err == nil {
		t.Fatal("缺 <item> 应报错")
	}
	// 非零 code → 错误
	if _, err := parseItemAttrs([]byte(`<ret code="-2" msg="鉴权失败"></ret>`)); err == nil {
		t.Fatal("非零 code 应报错")
	}
}

func TestParsePassport(t *testing.T) {
	m := parsePassport("userid=123456789|signvalid=abc123||c=3")
	if m["signvalid"] != "abc123" {
		t.Fatalf("signvalid=%q", m["signvalid"])
	}
	if m["userid"] != "123456789" {
		t.Fatalf("userid=%q", m["userid"])
	}
	if m["c"] != "3" {
		t.Fatalf("c=%q", m["c"])
	}
	if _, ok := parsePassport("")["signvalid"]; ok {
		t.Fatal("空 blob 不该有 signvalid")
	}
}

func TestParseCookieString(t *testing.T) {
	// 合成 id,非真实账号。
	m := parseCookieString("u_ukey=A1; userid=123456789; sess_tk=eyJ.x.y")
	if m["userid"] != "123456789" || m["u_ukey"] != "A1" || m["sess_tk"] != "eyJ.x.y" {
		t.Fatalf("got %+v", m)
	}
	// 值里含 '=' 的须完整保留(URL 编码的 value 常见)。
	m2 := parseCookieString("u_dpass=a%3D%3Db; x=1")
	if m2["u_dpass"] != "a%3D%3Db" {
		t.Fatalf("u_dpass=%q", m2["u_dpass"])
	}
}

func TestParseSetCookieHeader(t *testing.T) {
	// ⚠️ 实测 upass 会回**逗号分隔多 cookie**;每枚只取第一个 key=value(丢弃 Path 等属性)。
	h := "sess_tk=eyJ.x.y; Path=/; HttpOnly, utk=abc; Path=/; Domain=.10jqka.com.cn"
	m := parseSetCookieHeader(h)
	if m["sess_tk"] != "eyJ.x.y" {
		t.Fatalf("sess_tk=%q", m["sess_tk"])
	}
	if m["utk"] != "abc" {
		t.Fatalf("utk=%q", m["utk"])
	}
}

func TestJWTExpiry(t *testing.T) {
	// 造 payload {"exp":1893456000} = 2030-01-01Z
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"jti":"x","exp":1893456000}`))
	tok := "eyJhbGciOiJFUzI1NiJ9." + payload + ".sig"
	exp, ok := jwtExpiry(tok)
	if !ok {
		t.Fatal("应解析出 exp")
	}
	if exp.Unix() != 1893456000 {
		t.Fatalf("exp=%d", exp.Unix())
	}
	for _, bad := range []string{"", "notajwt", "a.b", "a.b.c.d", "x." + base64.RawURLEncoding.EncodeToString([]byte(`{}`)) + ".z"} {
		if _, ok := jwtExpiry(bad); ok {
			t.Fatalf("非法 JWT %q 不该解析成功", bad)
		}
	}
}

func TestDecodeDetailBlob(t *testing.T) {
	// 真实上游形状(实测片段)
	raw := `[{"M":"17","C":"601091","P":"37.85","T":"20260922"},{"M":"33","C":"002202","P":"18.13","T":"20260921"}]`
	blob := base64.StdEncoding.EncodeToString([]byte(raw))
	got, err := decodeDetailBlob(blob)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].C != "601091" || got[0].P != "37.85" || got[0].T != "20260922" {
		t.Fatalf("got %+v", got)
	}
	if got[1].M != "33" {
		t.Fatalf("M=%q", got[1].M)
	}

	// 空串 → 空切片,非错误(上游可能真没元数据)
	if got, err := decodeDetailBlob(""); err != nil || got != nil {
		t.Fatalf("空串应得 (nil,nil), got (%v,%v)", got, err)
	}
	// 坏 base64 → 错误(结构漂移,不空成功)
	if _, err := decodeDetailBlob("!!!not base64!!!"); err == nil {
		t.Fatal("坏 base64 应报错")
	}
	// 解出非 JSON → 错误
	if _, err := decodeDetailBlob(base64.StdEncoding.EncodeToString([]byte("not json"))); err == nil {
		t.Fatal("坏 JSON 应报错")
	}
}

func TestParseAddedPrice(t *testing.T) {
	cases := []struct {
		in      string
		wantNil bool
		want    float64
	}{
		{"37.85", false, 37.85},
		{"18930", false, 18930},
		{"", true, 0},
		{"0", true, 0},   // 0 不是有效 A 股价格 → NULL,不填 0
		{"-1", true, 0},  // 负值 → NULL
		{"abc", true, 0}, // 不可解析 → NULL
		{"  ", true, 0},  // 纯空白 → NULL
	}
	for _, c := range cases {
		got, ok := ParseAddedPrice(c.in)
		if c.wantNil {
			if ok || got != nil {
				t.Fatalf("ParseAddedPrice(%q) 应得 nil, got (%v,%v)", c.in, got, ok)
			}
			continue
		}
		if !ok || got == nil || *got != c.want {
			t.Fatalf("ParseAddedPrice(%q) = (%v,%v), want %v", c.in, got, ok, c.want)
		}
	}
}

func TestParseAddedOn(t *testing.T) {
	got, ok := ParseAddedOn("20260922")
	if !ok || got.Format("2006-01-02") != "2026-09-22" {
		t.Fatalf("上游格式解析失败: %v %v", got, ok)
	}
	got, ok = ParseAddedOn("2026-09-22")
	if !ok || got.Format("2006-01-02") != "2026-09-22" {
		t.Fatalf("ISO 格式解析失败: %v %v", got, ok)
	}
	for _, bad := range []string{"", "0", "abc", "2026/09/22", "202609"} {
		if _, ok := ParseAddedOn(bad); ok {
			t.Fatalf("非法日期 %q 不该解析成功", bad)
		}
	}
}

func TestMarketAbbr(t *testing.T) {
	cases := map[string]string{
		"17": "SH", "33": "SZ", "18": "KC", "38": "CYB", "71": "BJ", "151": "BJ",
		"48": "ZS", "55": "HK", "61": "US", "65": "65", // 未知码原样返回
	}
	for in, want := range cases {
		if got := MarketAbbr(in); got != want {
			t.Fatalf("MarketAbbr(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestIsAShare(t *testing.T) {
	for _, id := range []string{"17", "33", "18", "38", "71", "151"} {
		if !IsAShare(id) {
			t.Fatalf("IsAShare(%q) 应为 true", id)
		}
	}
	// 实测出现的非 A 股码(指数/港美/期货/自定义)
	for _, id := range []string{"48", "88", "55", "61", "65", "UCXF", "218", "97", "219", ""} {
		if IsAShare(id) {
			t.Fatalf("IsAShare(%q) 应为 false", id)
		}
	}
}

func TestParseRealheadName(t *testing.T) {
	body := []byte(`quotebridge_v6_realhead_hs_601091_last({"items":{"name":"C沈鼓","10":"37.98"}})`)
	name, ok := parseRealheadName(body)
	if !ok || name != "C沈鼓" {
		t.Fatalf("got (%q,%v)", name, ok)
	}
	// 无 JSONP 包裹 / 无 name → false(尽力而为,不报错)
	for _, bad := range []string{`no json here`, `fn({"items":{}})`, ``} {
		if _, ok := parseRealheadName([]byte(bad)); ok {
			t.Fatalf("%q 不该解析出名字", bad)
		}
	}
}

func TestEncryptPKCS1v15(t *testing.T) {
	// 测试内生成密钥对(不引新依赖,全 stdlib),验证加密结果可被同私钥解出。
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))

	enc, err := encryptPKCS1v15(pemStr, "my-account")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := rsa.DecryptPKCS1v15(rand.Reader, priv, raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "my-account" {
		t.Fatalf("往返不一致: %q", plain)
	}

	// 非法 PEM → 错误
	if _, err := encryptPKCS1v15("not pem", "x"); err == nil {
		t.Fatal("非法 PEM 应报错")
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

// 编译期断言:beijing 是 +08:00(加入日按北京日历解析,防跨日漂移)。
var _ = func() bool {
	_, off := time.Now().In(beijing).Zone()
	return off == 8*3600
}()

// TestXMLCharsetReader 回归:上游 XML 声明 encoding="GB2312"(实测原文),
// 裸 xml.NewDecoder 会报 `encoding "GB2312" declared but Decoder.CharsetReader is nil`
// —— Phase A 探针实测复现,verify2 第一步就倒。newXMLDecoder 必须能解。
//
// 两个真实场景:① 纯 ASCII 的 do_rsa(公钥 PEM 在属性里);② 真·GBK 中文(错误 msg)。
func TestXMLCharsetReader(t *testing.T) {
	// ① do_rsa 原样(声明 GB2312,内容纯 ASCII,属性内 PEM 含换行须原样保留)。
	const dorsa = "<?xml version=\"1.0\" encoding=\"GB2312\"?>\r\n" +
		"<do_rsa>\r\n<ret code=\"0\" msg=\"\"/>\r\n" +
		"<item rsa_version=\"default_5\" pubkey=\"-----BEGIN PUBLIC KEY-----\n" +
		"AAAABBBBCCCC\n-----END PUBLIC KEY-----\" modulus=\"D90F\"/>\r\n</do_rsa>"
	se, err := firstStartElement([]byte(dorsa), "item")
	if err != nil {
		t.Fatalf("GB2312 声明应能被解: %v", err)
	}
	var pub, ver string
	for _, a := range se.Attr {
		switch a.Name.Local {
		case "pubkey":
			pub = a.Value
		case "rsa_version":
			ver = a.Value
		}
	}
	if ver != "default_5" {
		t.Fatalf("rsa_version=%q", ver)
	}
	if !strings.Contains(pub, "-----BEGIN PUBLIC KEY-----\n") {
		t.Fatalf("属性内 PEM 换行被破坏: %q", pub)
	}

	// ② 真 GBK 中文 msg(GB2312 家族的常见真实内容)—— 须正确转成 UTF-8。
	gbkRaw, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(
		"<?xml version=\"1.0\" encoding=\"GB2312\"?><download><ret code=\"-1\" msg=\"账号或密码错误\"/></download>"))
	if err != nil {
		t.Fatal(err)
	}
	gbkBody := string(gbkRaw)
	code, msg, err := parseRet([]byte(gbkBody))
	if err == nil {
		t.Fatal("非零 code 应报错")
	}
	if code != "-1" || msg != "账号或密码错误" {
		t.Fatalf("GBK 解码错误: code=%q msg=%q", code, msg)
	}
}
