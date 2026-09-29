
## Go 代码风格规则

### 核心原则

清晰 > 简约 > 简洁 > 可维护 > 一致。

```go
// Good: 单次使用直接内联
journal, _ := os.ReadFile(filepath.Join(dir, "journal.json"))

// Bad: 多余中间变量
journalPath := filepath.Join(dir, "journal.json")
journal, _ := os.ReadFile(journalPath)
```

### 控制流

避免 `else`，优先提前返回。

```go
// Good
func foo(c bool) int {
    if c { return 1 }
    return 2
}

// Bad
func foo(c bool) int {
    if c { return 1 } else { return 2 }
}
```

### 变量

优先 `:=`，避免不必要的 `var` 和 `else` 重赋值。

```go
// Good
foo := 1
if c { foo = 2 }

// Bad
var foo int
if c { foo = 2 } else { foo = 1 }
```

### 复杂逻辑

主函数读作 happy path，辅助函数放下面并按概念命名。

```go
// Good
func LoadThing(input []byte) (*Thing, error) {
    cfg, err := requireConfig(input)
    if err != nil { return nil, err }
    meta := readMetadata(input)
    return createThing(cfg, meta), nil
}

func requireConfig(input []byte) (*Config, error) { ... }
```

### 导入

必须 `gofmt`；不用点导入；避免别名导入。

```go
// Good
import (
    "fmt"
    "os"

    "github.com/x/y"
)

// Bad
import . "fmt"
import z "github.com/x/y"
```

### 命名

包名简短小写；接收者 1–2 字母；缩写全大写。

```go
// Good
package tabwriter
type URLParser struct{}
func (s *Server) Serve() {}

// Bad
package tabWriter
type UrlParser struct{}
func (this *Server) Serve() {}
```

### 注释

导出标识符必须有文档注释，首句以名称开头、句号结尾。

```go
// Good
// ParseURL parses raw into a URL.
func ParseURL(raw string) (*URL, error) {}

// Bad
// this function parses url
func ParseURL(raw string) (*URL, error) {}
```

### 包与模块

包小而专注；内部实现放 `internal/`；`go.mod` 只含一个 `module` 指令，位于首行。

```
module github.com/you/proj

go 1.22

require github.com/x/y v1.0.0
```

### 错误处理

普通错误用 `return`；错误字符串小写、无结尾标点。

```go
// Good
return fmt.Errorf("open config: %w", err)

// Bad
return errors.New("Failed to open config.")
```

### 并发与上下文

`ctx` 是可能阻塞函数的第一个参数；含 `sync.Mutex` 的结构体方法用指针接收者。

```go
// Good
func (s *Store) Get(ctx context.Context, id string) (*Item, error) {}

// Bad
func (s Store) Get(id string, ctx context.Context) (*Item, error) {}
```

### 测试

不用 `assert` 库；失败输出 got 在前、want 在后。
```go
// Good
if got := Add(1, 2); got != 3 {
    t.Errorf("Add(1, 2) = %d, want %d", got, 3)
}
```

测试行为，不以覆盖率为目标。只写关键等价类和边界 case；每个 case 必须能说明捕获什么回归。覆盖率只用于发现盲区，不设硬性百分比。
```go
// Good: 关键等价类和边界，而不是穷举
tests := []struct {
    name    string
    in      string
    want    int
    wantErr bool
}{
    {"empty uses default", "", 10, false},
    {"valid", "42", 42, false},
    {"zero invalid", "0", 0, true},
    {"negative invalid", "-1", 0, true},
    {"not a number", "abc", 0, true},
}

// Bad: 穷举
// Bad: 为覆盖率穷举，没有新增行为
for _, s := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"} {
    // ...
}
```

### 数据与 Schema

列名 `snake_case`，Go 字段 `CamelCase`，通过 struct tag 映射。

```go
// Good
type Session struct {
    ID        string `db:"id"`
    ProjectID string `db:"project_id"`
    CreatedAt int64  `db:"created_at"`
}
```

### 其他

空切片用 `var s []int`；安全敏感场景用 `crypto/rand`。

```go
// Good
var items []Item

// Bad
items := []Item{}
```

## Branch Names

Use a short branch name of at most three words, separated by hyphens. Do not use slashes or type prefixes such as `feat/` or `fix/`.

Examples: `session-recovery`, `fix-scroll-state`, `regenerate-sdk`.

## Commits and PR Titles

Use conventional commit-style messages and PR titles: `type(scope): summary`.

Valid types are `feat`, `fix`, `docs`, `chore`, `refactor`, and `test`. Scopes are optional; use the affected package or area when helpful, e.g. `core`, `opencode`, `tui`, `app`, `desktop`, `sdk`, or `plugin`.

Examples: `fix(tui): simplify thinking toggle styling`, `docs: update contributing guide`, `chore(sdk): regenerate types`.


## 标识值命名约定
我们自己定义的标识值（资源 ID、计费项 code、枚举值、命名空间、幂等键）统一用 : 分段拼接，其余数据库/代码字段、路径/URL、环境变量、HTTP header、第三方协议值及受外部格式约束的值保持原样。
```go
// Good
const UsageEventRefType = "usage:event"

// Bad
const UsageEventRefType = "usage_event"
```
