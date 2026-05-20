package services

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"backend_axenta/models"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type mockAuth struct {
	calls int32
	exp   time.Time
	fail  bool
}

func (m *mockAuth) Authenticate(_ context.Context, login, password string) (*TenantCredentials, error) {
	n := atomic.AddInt32(&m.calls, 1)
	if m.fail {
		return nil, fmt.Errorf("axenta down")
	}
	return &TenantCredentials{
		Login: login, Password: password,
		Token:     fmt.Sprintf("tok-%s-%d", login, n),
		ExpiresAt: m.exp,
	}, nil
}

func astDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Company{}))
	return db
}

func mkCompany(t *testing.T, db *gorm.DB, id uint, login, pass string) {
	require.NoError(t, db.Create(&models.Company{
		ID: id, Name: "C", DatabaseSchema: fmt.Sprintf("tenant_%d", id),
		Domain: fmt.Sprintf("c%d", id), AxetnaLogin: login,
		AxetnaPassword: pass, IsActive: true, CompanyType: "partner",
	}).Error)
}

func TestAxentaServerToken_AcquireAndCache(t *testing.T) {
	db := astDB(t)
	mkCompany(t, db, 1, "acc1", "pwd1")
	m := &mockAuth{exp: time.Now().Add(time.Hour)}
	svc := NewAxentaServerToken(db, m)

	tok, err := svc.Token(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "tok-acc1-1", tok)

	tok2, err := svc.Token(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, tok, tok2)
	require.EqualValues(t, 1, atomic.LoadInt32(&m.calls), "кэш → без повторного auth")
}

func TestAxentaServerToken_NoCreds(t *testing.T) {
	db := astDB(t)
	mkCompany(t, db, 2, "", "")
	svc := NewAxentaServerToken(db, &mockAuth{exp: time.Now().Add(time.Hour)})
	_, err := svc.Token(context.Background(), 2)
	require.ErrorIs(t, err, ErrNoAxentaCreds)
}

func TestAxentaServerToken_InvalidateRefetches(t *testing.T) {
	db := astDB(t)
	mkCompany(t, db, 3, "acc3", "p")
	m := &mockAuth{exp: time.Now().Add(time.Hour)}
	svc := NewAxentaServerToken(db, m)

	_, _ = svc.Token(context.Background(), 3)
	svc.Invalidate(3)
	tok, err := svc.Token(context.Background(), 3)
	require.NoError(t, err)
	require.Equal(t, "tok-acc3-2", tok)
	require.EqualValues(t, 2, atomic.LoadInt32(&m.calls))
}

func TestAxentaServerToken_ExpiredRefetches(t *testing.T) {
	db := astDB(t)
	mkCompany(t, db, 4, "acc4", "p")
	m := &mockAuth{exp: time.Now().Add(10 * time.Second)} // < skew(60s) → сразу «истёк»
	svc := NewAxentaServerToken(db, m)

	_, _ = svc.Token(context.Background(), 4)
	_, _ = svc.Token(context.Background(), 4)
	require.EqualValues(t, 2, atomic.LoadInt32(&m.calls), "в пределах skew → перелогин")
}

func TestAxentaServerToken_SingleflightNoStampede(t *testing.T) {
	db := astDB(t)
	mkCompany(t, db, 5, "acc5", "p")
	m := &mockAuth{exp: time.Now().Add(time.Hour)}
	svc := NewAxentaServerToken(db, m)

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = svc.Token(context.Background(), 5) }()
	}
	wg.Wait()
	require.EqualValues(t, 1, atomic.LoadInt32(&m.calls), "singleflight: 1 auth на стадо")
}
