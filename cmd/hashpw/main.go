// hashpw 本地小工具:把明文口令转成 bcrypt 哈希,供写入 lab .env 的
// PIKS_AUTH_PASSWORD_HASH(P12 / issue #78;设计 §3.3)。
//
// 用法(本地,勿在生产):
//
//	go run ./cmd/hashpw '你的口令'
//
// 输出形如 $2a$10$... —— 整行贴进 lab /home/rguo/piks/.env 的
// PIKS_AUTH_PASSWORD_HASH=$(...)。明文口令不经任何存储/日志。
//
// ⚠️ 本工具**不参与部署**(不属四镜像任一 target,不进任何常驻服务);
// web 只做 CompareHashAndPassword,绝不落明文。
package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] == "" {
		fmt.Fprintln(os.Stderr, "用法: go run ./cmd/hashpw '<口令>'")
		os.Exit(2)
	}
	h, err := bcrypt.GenerateFromPassword([]byte(os.Args[1]), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintln(os.Stderr, "生成失败:", err)
		os.Exit(1)
	}
	fmt.Println("PIKS_AUTH_PASSWORD_HASH=" + string(h))
}
