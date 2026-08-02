package queue

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/bitmagnet-io/bitmagnet/internal/database/dao"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func scopeSQL(t *testing.T, contentTypes []model.NullContentType) string {
	t.Helper()

	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}

	t.Cleanup(func() { _ = sqlDB.Close() })

	db, err := gorm.Open(
		postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}),
		&gorm.Config{DryRun: true},
	)
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}

	q := dao.Use(db)

	// apply the scope exactly as the handler does
	stmt := q.Torrent.WithContext(context.Background()).
		Scopes(ContentTypeScope(q, contentTypes)).
		UnderlyingDB()

	return stmt.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return tx.Find(&[]model.Torrent{})
	})
}

func TestContentTypeScopeSQL(t *testing.T) {
	t.Parallel()

	t.Run("null only stays correlated", func(t *testing.T) {
		t.Parallel()

		sql := scopeSQL(t, []model.NullContentType{{}})
		if !strings.Contains(sql, "content_type\" IS NULL") {
			t.Errorf("expected IS NULL predicate, got: %s", sql)
		}
		// the correlation MUST survive; without it EXISTS matches every torrent
		if !strings.Contains(sql, "info_hash\" = \"torrents\".\"info_hash\"") {
			t.Errorf("correlation lost — this is the bug that queued the whole table: %s", sql)
		}

		if strings.Contains(sql, "IN ()") || strings.Contains(sql, "IN (NULL)") {
			t.Errorf("empty IN generated: %s", sql)
		}
	})

	t.Run("mixed types are grouped, not ORed against the correlation", func(t *testing.T) {
		t.Parallel()

		sql := scopeSQL(t, []model.NullContentType{
			{},
			{Valid: true, ContentType: model.ContentTypeMovie},
		})
		if !strings.Contains(sql, "info_hash\" = \"torrents\".\"info_hash\"") {
			t.Errorf("correlation lost: %s", sql)
		}
		// the OR must be inside parentheses alongside the correlation, so an
		// AND still binds it
		if !strings.Contains(sql, "OR") {
			t.Errorf("expected an OR for the mixed case: %s", sql)
		}

		if !strings.Contains(sql, "AND") {
			t.Errorf("correlation must be ANDed with the grouped predicate: %s", sql)
		}
	})
}
