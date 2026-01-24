package ai

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/liuscraft/orion-x/internal/logging"
)

// MockTools 返回 100 个 mock 工具
func MockTools() []toolConfig {
	return []toolConfig{
		// 时间相关 (1-10)
		{name: "getCurrentTime", desc: "获取当前时间", params: map[string]string{}},
		{name: "getCurrentDate", desc: "获取当前日期", params: map[string]string{}},
		{name: "getTimestamp", desc: "获取当前时间戳", params: map[string]string{}},
		{name: "getWeekday", desc: "获取今天是星期几", params: map[string]string{}},
		{name: "getTimezone", desc: "获取当前时区", params: map[string]string{}},
		{name: "getUTCNow", desc: "获取当前UTC时间", params: map[string]string{}},
		{name: "getYear", desc: "获取当前年份", params: map[string]string{}},
		{name: "getMonth", desc: "获取当前月份", params: map[string]string{}},
		{name: "getDay", desc: "获取当前日期", params: map[string]string{}},
		{name: "isWeekend", desc: "判断今天是否是周末", params: map[string]string{}},

		// 天气相关 (11-20)
		{name: "getWeather", desc: "获取指定城市的天气", params: map[string]string{"city": "城市名称"}},
		{name: "getTemperature", desc: "获取指定城市的温度", params: map[string]string{"city": "城市名称"}},
		{name: "getHumidity", desc: "获取指定城市的湿度", params: map[string]string{"city": "城市名称"}},
		{name: "getWindSpeed", desc: "获取指定城市的风速", params: map[string]string{"city": "城市名称"}},
		{name: "getAirQuality", desc: "获取指定城市的空气质量", params: map[string]string{"city": "城市名称"}},
		{name: "getUVIndex", desc: "获取指定城市的紫外线指数", params: map[string]string{"city": "城市名称"}},
		{name: "getWeatherAlert", desc: "获取指定城市的天气预警", params: map[string]string{"city": "城市名称"}},
		{name: "getSunrise", desc: "获取指定城市的日出时间", params: map[string]string{"city": "城市名称"}},
		{name: "getSunset", desc: "获取指定城市的日落时间", params: map[string]string{"city": "城市名称"}},
		{name: "getForecast", desc: "获取指定城市未来几天的天气预报", params: map[string]string{"city": "城市名称", "days": "天数"}},

		// 地理相关 (21-30)
		{name: "getCountry", desc: "获取指定城市的国家", params: map[string]string{"city": "城市名称"}},
		{name: "getCapital", desc: "获取指定国家的首都", params: map[string]string{"country": "国家名称"}},
		{name: "getPopulation", desc: "获取指定城市的人口", params: map[string]string{"city": "城市名称"}},
		{name: "getArea", desc: "获取指定城市的面积", params: map[string]string{"city": "城市名称"}},
		{name: "getCurrency", desc: "获取指定国家的货币", params: map[string]string{"country": "国家名称"}},
		{name: "getLanguage", desc: "获取指定国家的官方语言", params: map[string]string{"country": "国家名称"}},
		{name: "getContinent", desc: "获取指定国家所在的大洲", params: map[string]string{"country": "国家名称"}},
		{name: "getCoordinate", desc: "获取指定城市的经纬度坐标", params: map[string]string{"city": "城市名称"}},
		{name: "getDistance", desc: "计算两个城市之间的距离", params: map[string]string{"city1": "城市1", "city2": "城市2"}},
		{name: "getTimezoneByCity", desc: "获取指定城市的时区", params: map[string]string{"city": "城市名称"}},

		// 计算相关 (31-40)
		{name: "calculateAdd", desc: "加法计算", params: map[string]string{"a": "第一个数", "b": "第二个数"}},
		{name: "calculateSubtract", desc: "减法计算", params: map[string]string{"a": "被减数", "b": "减数"}},
		{name: "calculateMultiply", desc: "乘法计算", params: map[string]string{"a": "第一个数", "b": "第二个数"}},
		{name: "calculateDivide", desc: "除法计算", params: map[string]string{"a": "被除数", "b": "除数"}},
		{name: "calculatePower", desc: "幂运算", params: map[string]string{"base": "底数", "exponent": "指数"}},
		{name: "calculateSqrt", desc: "计算平方根", params: map[string]string{"number": "数值"}},
		{name: "calculatePercentage", desc: "计算百分比", params: map[string]string{"part": "部分值", "total": "总值"}},
		{name: "calculateAverage", desc: "计算平均值", params: map[string]string{"numbers": "数字列表"}},
		{name: "calculateMax", desc: "找出最大值", params: map[string]string{"numbers": "数字列表"}},
		{name: "calculateMin", desc: "找出最小值", params: map[string]string{"numbers": "数字列表"}},

		// 数据查询相关 (41-50)
		{name: "searchWeb", desc: "在网络上搜索信息", params: map[string]string{"query": "搜索关键词"}},
		{name: "getNews", desc: "获取最新新闻", params: map[string]string{"category": "新闻类别"}},
		{name: "getStockPrice", desc: "获取股票价格", params: map[string]string{"symbol": "股票代码"}},
		{name: "getCryptoPrice", desc: "获取加密货币价格", params: map[string]string{"coin": "币种"}},
		{name: "getExchangeRate", desc: "获取汇率", params: map[string]string{"from": "源货币", "to": "目标货币"}},
		{name: "getCompanyInfo", desc: "获取公司信息", params: map[string]string{"company": "公司名称"}},
		{name: "getProductPrice", desc: "获取商品价格", params: map[string]string{"product": "商品名称"}},
		{name: "getMovieInfo", desc: "获取电影信息", params: map[string]string{"title": "电影标题"}},
		{name: "getBookInfo", desc: "获取书籍信息", params: map[string]string{"title": "书名"}},
		{name: "getDefinition", desc: "获取单词定义", params: map[string]string{"word": "单词"}},

		// 技术相关 (51-60)
		{name: "generateCode", desc: "生成代码", params: map[string]string{"language": "编程语言", "description": "功能描述"}},
		{name: "explainCode", desc: "解释代码", params: map[string]string{"code": "代码内容", "language": "编程语言"}},
		{name: "debugCode", desc: "调试代码找出错误", params: map[string]string{"code": "代码内容", "language": "编程语言"}},
		{name: "formatCode", desc: "格式化代码", params: map[string]string{"code": "代码内容", "language": "编程语言"}},
		{name: "reviewCode", desc: "代码审查", params: map[string]string{"code": "代码内容", "language": "编程语言"}},
		{name: "convertCode", desc: "代码语言转换", params: map[string]string{"code": "代码内容", "from": "源语言", "to": "目标语言"}},
		{name: "optimizeCode", desc: "优化代码性能", params: map[string]string{"code": "代码内容", "language": "编程语言"}},
		{name: "addComments", desc: "为代码添加注释", params: map[string]string{"code": "代码内容", "language": "编程语言"}},
		{name: "findBugs", desc: "查找代码中的bug", params: map[string]string{"code": "代码内容", "language": "编程语言"}},
		{name: "writeTests", desc: "为代码编写测试", params: map[string]string{"code": "代码内容", "language": "编程语言"}},

		// 文本处理相关 (61-70)
		{name: "summarizeText", desc: "总结文本内容", params: map[string]string{"text": "文本内容"}},
		{name: "translateText", desc: "翻译文本", params: map[string]string{"text": "文本内容", "target": "目标语言"}},
		{name: "detectLanguage", desc: "检测文本语言", params: map[string]string{"text": "文本内容"}},
		{name: "sentimentAnalysis", desc: "分析文本情感倾向", params: map[string]string{"text": "文本内容"}},
		{name: "extractKeywords", desc: "提取文本关键词", params: map[string]string{"text": "文本内容"}},
		{name: "countWords", desc: "统计文本字数", params: map[string]string{"text": "文本内容"}},
		{name: "rewriteText", desc: "改写文本", params: map[string]string{"text": "文本内容", "style": "改写风格"}},
		{name: "checkGrammar", desc: "检查文本语法错误", params: map[string]string{"text": "文本内容"}},
		{name: "extractEntities", desc: "提取文本中的实体", params: map[string]string{"text": "文本内容"}},
		{name: "generateTitle", desc: "为文本生成标题", params: map[string]string{"text": "文本内容"}},

		// 邮件相关 (71-75)
		{name: "sendEmail", desc: "发送邮件", params: map[string]string{"to": "收件人", "subject": "主题", "body": "正文"}},
		{name: "readEmail", desc: "读取邮件", params: map[string]string{"folder": "文件夹", "limit": "数量限制"}},
		{name: "searchEmail", desc: "搜索邮件", params: map[string]string{"query": "搜索关键词"}},
		{name: "deleteEmail", desc: "删除邮件", params: map[string]string{"id": "邮件ID"}},
		{name: "archiveEmail", desc: "归档邮件", params: map[string]string{"id": "邮件ID"}},

		// 日历相关 (76-80)
		{name: "createEvent", desc: "创建日历事件", params: map[string]string{"title": "标题", "start": "开始时间", "end": "结束时间"}},
		{name: "getEvents", desc: "获取日历事件", params: map[string]string{"date": "日期", "limit": "数量限制"}},
		{name: "updateEvent", desc: "更新日历事件", params: map[string]string{"id": "事件ID", "title": "标题"}},
		{name: "deleteEvent", desc: "删除日历事件", params: map[string]string{"id": "事件ID"}},
		{name: "searchEvent", desc: "搜索日历事件", params: map[string]string{"query": "搜索关键词"}},

		// 文件相关 (81-90)
		{name: "readFile", desc: "读取文件内容", params: map[string]string{"path": "文件路径"}},
		{name: "writeFile", desc: "写入文件内容", params: map[string]string{"path": "文件路径", "content": "内容"}},
		{name: "deleteFile", desc: "删除文件", params: map[string]string{"path": "文件路径"}},
		{name: "copyFile", desc: "复制文件", params: map[string]string{"source": "源路径", "destination": "目标路径"}},
		{name: "moveFile", desc: "移动文件", params: map[string]string{"source": "源路径", "destination": "目标路径"}},
		{name: "listFiles", desc: "列出目录下的文件", params: map[string]string{"path": "目录路径"}},
		{name: "searchFiles", desc: "搜索文件", params: map[string]string{"query": "搜索关键词", "path": "搜索路径"}},
		{name: "compressFile", desc: "压缩文件", params: map[string]string{"source": "源路径", "destination": "目标路径"}},
		{name: "decompressFile", desc: "解压文件", params: map[string]string{"source": "源路径", "destination": "目标路径"}},
		{name: "getFileInfo", desc: "获取文件信息", params: map[string]string{"path": "文件路径"}},

		// 数据库相关 (91-100)
		{name: "queryDatabase", desc: "查询数据库", params: map[string]string{"table": "表名", "conditions": "查询条件"}},
		{name: "insertRecord", desc: "插入记录", params: map[string]string{"table": "表名", "data": "数据"}},
		{name: "updateRecord", desc: "更新记录", params: map[string]string{"table": "表名", "id": "记录ID", "data": "数据"}},
		{name: "deleteRecord", desc: "删除记录", params: map[string]string{"table": "表名", "id": "记录ID"}},
		{name: "countRecords", desc: "统计记录数量", params: map[string]string{"table": "表名"}},
		{name: "executeQuery", desc: "执行SQL查询", params: map[string]string{"sql": "SQL语句"}},
		{name: "backupDatabase", desc: "备份数据库", params: map[string]string{"database": "数据库名"}},
		{name: "restoreDatabase", desc: "恢复数据库", params: map[string]string{"database": "数据库名", "backup": "备份文件"}},
		{name: "getTableSchema", desc: "获取表结构", params: map[string]string{"table": "表名"}},
		{name: "exportData", desc: "导出数据", params: map[string]string{"table": "表名", "format": "导出格式"}},
	}
}

type toolConfig struct {
	name   string
	desc   string
	params map[string]string
}

// CreateMockTools 从配置创建实际的工具实例
func CreateMockTools() ([]interface{}, error) {
	configs := MockTools()

	now := time.Now()
	tools := make([]interface{}, 0, len(configs))

	for _, cfg := range configs {
		var t interface{}
		var err error

		switch cfg.name {
		// 时间相关
		case "getCurrentTime":
			t, _ = utils.InferTool(cfg.name, cfg.desc, func(context.Context, struct{}) (string, error) {
				result := now.Format("2006-01-02 15:04:05")
				logging.Infof("[Tool] %s -> %s", cfg.name, result)
				return result, nil
			})
		case "getCurrentDate":
			t, _ = utils.InferTool(cfg.name, cfg.desc, func(context.Context, struct{}) (string, error) {
				result := now.Format("2006-01-02")
				logging.Infof("[Tool] %s -> %s", cfg.name, result)
				return result, nil
			})
		case "getTimestamp":
			t, _ = utils.InferTool(cfg.name, cfg.desc, func(context.Context, struct{}) (string, error) {
				result := fmt.Sprintf("%d", now.Unix())
				logging.Infof("[Tool] %s -> %s", cfg.name, result)
				return result, nil
			})
		case "getWeekday":
			t, _ = utils.InferTool(cfg.name, cfg.desc, func(context.Context, struct{}) (string, error) {
				result := now.Weekday().String()
				logging.Infof("[Tool] %s -> %s", cfg.name, result)
				return result, nil
			})
		case "getTimezone":
			t, _ = utils.InferTool(cfg.name, cfg.desc, func(context.Context, struct{}) (string, error) {
				result := now.Location().String()
				logging.Infof("[Tool] %s -> %s", cfg.name, result)
				return result, nil
			})
		case "getUTCNow":
			t, _ = utils.InferTool(cfg.name, cfg.desc, func(context.Context, struct{}) (string, error) {
				result := now.UTC().Format("2006-01-02 15:04:05 UTC")
				logging.Infof("[Tool] %s -> %s", cfg.name, result)
				return result, nil
			})
		case "getYear":
			t, _ = utils.InferTool(cfg.name, cfg.desc, func(context.Context, struct{}) (string, error) {
				result := fmt.Sprintf("%d", now.Year())
				logging.Infof("[Tool] %s -> %s", cfg.name, result)
				return result, nil
			})
		case "getMonth":
			t, _ = utils.InferTool(cfg.name, cfg.desc, func(context.Context, struct{}) (string, error) {
				result := now.Format("01")
				logging.Infof("[Tool] %s -> %s", cfg.name, result)
				return result, nil
			})
		case "getDay":
			t, _ = utils.InferTool(cfg.name, cfg.desc, func(context.Context, struct{}) (string, error) {
				result := fmt.Sprintf("%d", now.Day())
				logging.Infof("[Tool] %s -> %s", cfg.name, result)
				return result, nil
			})
		case "isWeekend":
			t, _ = utils.InferTool(cfg.name, cfg.desc, func(context.Context, struct{}) (string, error) {
				result := fmt.Sprintf("%v", now.Weekday() == time.Saturday || now.Weekday() == time.Sunday)
				logging.Infof("[Tool] %s -> %s", cfg.name, result)
				return result, nil
			})

		// 天气相关 - 使用通用处理器
		default:
			// 为其他工具创建通用 mock 处理器
			t, err = createGenericMockTool(cfg)
			if err != nil {
				return nil, err
			}
		}

		if t != nil {
			tools = append(tools, t)
		}
	}

	return tools, nil
}

// createGenericMockTool 创建通用 mock 工具
func createGenericMockTool(cfg toolConfig) (interface{}, error) {
	t, _ := utils.InferTool(cfg.name, cfg.desc, func(ctx context.Context, args map[string]interface{}) (string, error) {
		logging.Infof("[Tool] %s called with args: %v", cfg.name, args)

		// 根据工具名称返回模拟结果
		switch {
		case cfg.name == "getWeather":
			city := "未知城市"
			if c, ok := args["city"].(string); ok {
				city = c
			}
			return fmt.Sprintf("%s的天气：晴天，温度25°C，湿度60%%", city), nil

		case cfg.name == "getTemperature":
			city := "未知城市"
			if c, ok := args["city"].(string); ok {
				city = c
			}
			return fmt.Sprintf("%s当前温度：25°C", city), nil

		case cfg.name == "calculateAdd":
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			return fmt.Sprintf("%.2f", a+b), nil

		case cfg.name == "calculateSubtract":
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			return fmt.Sprintf("%.2f", a-b), nil

		case cfg.name == "calculateMultiply":
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			return fmt.Sprintf("%.2f", a*b), nil

		case cfg.name == "calculateDivide":
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			if b == 0 {
				return "错误：除数不能为零", nil
			}
			return fmt.Sprintf("%.2f", a/b), nil

		case cfg.name == "searchWeb":
			query := "未知查询"
			if q, ok := args["query"].(string); ok {
				query = q
			}
			return fmt.Sprintf("关于'%s'的搜索结果：找到相关信息...", query), nil

		case cfg.name == "getStockPrice":
			symbol := "未知股票"
			if s, ok := args["symbol"].(string); ok {
				symbol = s
			}
			return fmt.Sprintf("%s 股票价格: ¥%.2f", symbol, 100.50), nil

		case cfg.name == "sendEmail":
			to := "未知收件人"
			if t, ok := args["to"].(string); ok {
				to = t
			}
			return fmt.Sprintf("邮件已发送给 %s", to), nil

		case cfg.name == "translateText":
			text := "未知文本"
			if t, ok := args["text"].(string); ok {
				text = t
			}
			target := "英文"
			if t, ok := args["target"].(string); ok {
				target = t
			}
			return fmt.Sprintf("'%s' 已翻译为 %s", text, target), nil

		case cfg.name == "summarizeText":
			text := "未知文本"
			if t, ok := args["text"].(string); ok {
				text = t[:min(20, len(text))]
			}
			return fmt.Sprintf("摘要：%s...", text), nil

		default:
			return fmt.Sprintf("%s 执行成功", cfg.name), nil
		}
	})
	return t, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
