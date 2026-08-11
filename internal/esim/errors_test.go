package esim

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsEUICCNotFound(t *testing.T) {
	if !IsEUICCNotFound(ErrNoEUICC) {
		t.Fatalf("IsEUICCNotFound(ErrNoEUICC) want true")
	}
	// 包装后的错误仍应被识别（errors.Is 链式判别）
	wrapped := fmt.Errorf("scan failed: %w", ErrNoEUICC)
	if !IsEUICCNotFound(wrapped) {
		t.Fatalf("IsEUICCNotFound(wrapped ErrNoEUICC) want true")
	}
	// 双重包装（保留底层错误链，模拟 forEachEUICC 的 %w:%w 返回）
	doubleWrapped := fmt.Errorf("%w: %w", ErrNoEUICC, errors.New("open logical channel: QMI error"))
	if !IsEUICCNotFound(doubleWrapped) {
		t.Fatalf("IsEUICCNotFound(double-wrapped ErrNoEUICC) want true")
	}
	// 无关错误不应被识别
	if IsEUICCNotFound(errors.New("other error")) {
		t.Fatalf("IsEUICCNotFound(other) want false")
	}
	if IsEUICCNotFound(nil) {
		t.Fatalf("IsEUICCNotFound(nil) want false")
	}
}
