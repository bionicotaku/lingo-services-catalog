// Package po 定义面向持久化的数据对象（Persistent Objects），由 Repository 层使用。
// PO 对象映射数据库表结构，不直接暴露给上层业务逻辑。
package po

// Greeter 表示持久化存储中的 Greeter 实体。
// 对应数据库表的字段结构（当前为示例，实际接入数据库时需补充字段）。
type Greeter struct {
	Hello string // 问候对象的名称
	// TODO: 接入数据库后添加字段，如：
	// ID        int64     `db:"id"`
	// CreatedAt time.Time `db:"created_at"`
	// UpdatedAt time.Time `db:"updated_at"`
}
