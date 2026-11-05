package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
)

type Box struct{ aead cipher.AEAD }

func NewBox(key []byte) (*Box, error) {
	b, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	a, e := cipher.NewGCM(b)
	return &Box{a}, e
}
func OpenBox(dir string) (*Box, error) {
	path := filepath.Join(dir, "master.key")
	key, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if info, e := os.Stat(filepath.Join(dir, "tingfeng.db")); e == nil && info.Size() > 0 {
			return nil, fmt.Errorf("已有数据库缺少 master.key，请从完整备份恢复密钥")
		}
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return nil, e
		}
		_, err = f.Write(key)
		closeErr := f.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
	} else if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("master.key 不完整，请从备份恢复；不能更换已有数据库的密钥")
	}
	return NewBox(key)
}
func (b *Box) Seal(data []byte) ([]byte, error) {
	n := make([]byte, b.aead.NonceSize())
	if _, e := rand.Read(n); e != nil {
		return nil, e
	}
	return b.aead.Seal(n, n, data, []byte("tingfeng-v1")), nil
}
func (b *Box) Open(data []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(data) < n {
		return nil, fmt.Errorf("加密配置已损坏")
	}
	p, e := b.aead.Open(nil, data[:n], data[n:], []byte("tingfeng-v1"))
	if e != nil {
		return nil, fmt.Errorf("无法解密配置，请检查 master.key 是否匹配")
	}
	return p, nil
}
