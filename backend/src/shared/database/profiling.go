package database

import (
	"gorm.io/gorm"

	"refratia/backend/src/shared/profiling"
)

// profileSQL times every gorm statement as sql.<op> (table and row count,
// never values) when profiling is on. Statements run on a transaction handle
// without a context carry no request tag; the summary attributes them by time.
func profileSQL(db *gorm.DB) error {
	if !profiling.Enabled() {
		return nil
	}
	const key = "profiling:span"
	start := func(op string) func(*gorm.DB) {
		return func(statement *gorm.DB) {
			statement.InstanceSet(key, profiling.Start(statement.Statement.Context, "sql."+op))
		}
	}
	end := func(statement *gorm.DB) {
		value, _ := statement.InstanceGet(key)
		span, _ := value.(*profiling.Span)
		span.Set("table", statement.Statement.Table).Set("rows", statement.RowsAffected).End(statement.Error)
	}
	callbacks := db.Callback()
	for _, err := range []error{
		callbacks.Create().Before("*").Register("profiling:before_create", start("create")),
		callbacks.Create().After("*").Register("profiling:after_create", end),
		callbacks.Query().Before("*").Register("profiling:before_query", start("query")),
		callbacks.Query().After("*").Register("profiling:after_query", end),
		callbacks.Update().Before("*").Register("profiling:before_update", start("update")),
		callbacks.Update().After("*").Register("profiling:after_update", end),
		callbacks.Delete().Before("*").Register("profiling:before_delete", start("delete")),
		callbacks.Delete().After("*").Register("profiling:after_delete", end),
		callbacks.Row().Before("*").Register("profiling:before_row", start("row")),
		callbacks.Row().After("*").Register("profiling:after_row", end),
		callbacks.Raw().Before("*").Register("profiling:before_raw", start("raw")),
		callbacks.Raw().After("*").Register("profiling:after_raw", end),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}
