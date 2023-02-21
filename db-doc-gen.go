package main

import (
	"database/sql"
	"flag"
	"fmt"
	_ "fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// 运行
func main() {

	genDoc()

	var realPath *string
	realPath = flag.String("path", ".", "doc resource path")
	flag.Parse()
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		path := request.URL.Path
		fmt.Printf("path: %s\n", path)
		requestType := path[strings.LastIndex(path, "."):]
		switch requestType {
		case ".doc":
			writer.Header().Set("content-type", "text/html")
		default:
		}
		fin, err := os.Open(*realPath + path)
		defer func(fin *os.File) {
			err := fin.Close()
			checkErr(err)
		}(fin)
		if err != nil {
			log.Fatal("doc resource path:", err)
		}
		fd, _ := io.ReadAll(fin)
		_, err = writer.Write(fd)
		checkErr(err)
	})

	http.HandleFunc("/gen-doc", func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		redirect := query.Get("redirect")
		genDoc()
		http.Redirect(writer, request, redirect, http.StatusFound)
	})
	server := http.Server{Addr: ":8080"}
	log.Println("running in port: 8080")
	err := server.ListenAndServe()
	checkErr(err)
}

// 生成 html 文档
func genDoc() {
	config := initConfig()
	datasources := config.Datasources
	fmt.Printf("datasources: %#v\n", datasources)
	var databases []Database
	for _, ds := range datasources {
		fmt.Printf("ds: %#v\n", ds)
		database := getDatabase(ds)
		databases = append(databases, database)
	}

	htmlPath := "./doc"
	_, err := os.Stat(htmlPath)
	if err == nil || os.IsExist(err) {
		err = os.RemoveAll(htmlPath)
		checkErr(err)
	}
	err = os.MkdirAll(htmlPath, 0755)
	checkErr(err)

	file, err := os.Create("./doc/doc.html")
	checkErr(err)
	tpl := template.Must(template.ParseGlob("views/*/*.html"))
	err = tpl.ExecuteTemplate(file,
		"default/doc",
		map[string]interface{}{
			"Databases": databases,
		},
	)
	checkErr(err)
}

// 初始化配置
func initConfig() *Config {
	viper.SetConfigType("yaml")
	viper.SetConfigFile("./config.yaml")
	err := viper.ReadInConfig()
	if err != nil {
		checkErr(err)
	}
	var _config *Config
	err = viper.Unmarshal(&_config)
	checkErr(err)
	fmt.Printf("config: %#v\n", _config)
	return _config
}

// 获取数据库字典
func getDatabase(dataSource Datasource) Database {
	db := initDb(dataSource)
	rows, err := db.Query("select s.schema_name, s.default_character_set_name, s.default_collation_name from information_schema.schemata s where s.schema_name = ?; ", dataSource.Name)
	checkErr(err)
	var database Database
	rows.Next()
	err = rows.Scan(
		&database.Name,
		&database.DefaultCharacterSetName,
		&database.DefaultCollationName,
	)
	checkErr(err)

	var tables []Table
	rows, err = db.Query("select t.table_name,t.table_comment, t.table_collation, t.create_time  from information_schema.tables t where t.table_schema = ?", dataSource.Name)
	checkErr(err)

	for rows.Next() {
		var table Table
		err := rows.Scan(
			&table.Name,
			&table.Comment,
			&table.CollationName,
			&table.CreateTime,
		)
		checkErr(err)

		rowsColumn, errColumn := db.Query("select     column_name '字段名称',     column_type '字段类型',     (case         when is_nullable = 'YES' then '是'         else '否'     end) '是否可空',     (case         when column_key = 'PRI' then '是'         else '否'     end) '是否主键',     character_set_name '字符编码',     collation_name '字符集名',     column_default '默认值',     extra '扩展值',     column_comment '注释' from     information_schema.columns where     table_schema = 'feedback-service'     and table_name = ?; ", "t_complaint")
		checkErr(errColumn)

		// 读取字段信息
		var columns []Column
		var column Column
		for rowsColumn.Next() {
			errColumn = rowsColumn.Scan(
				&column.Name,
				&column.Type,
				&column.Nullable,
				&column.Primary,
				&column.CharacterSetName,
				&column.CollationName,
				&column.Default,
				&column.Extra,
				&column.Comment)
			checkErr(errColumn)
			columns = append(columns, column)
		}
		table.Columns = columns
		tables = append(tables, table)
	}

	database.Tables = tables
	fmt.Println(database.Tables)

	// 关闭查询
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			checkErr(err)
		}
	}(rows)

	// 关闭数据库
	defer func(db *sql.DB) {
		err := db.Close()
		checkErr(err)
	}(db)
	return database
}

// 初始化数据库
func initDb(datasource Datasource) *sql.DB {
	dsn := fmt.Sprintf("%s:%s@%s(%s:%d)/%s", datasource.Username, datasource.Password, datasource.Protocol, datasource.Host, datasource.Port, datasource.Name)
	db, err := sql.Open("mysql", dsn)
	checkErr(err)
	// 设置数据库的最大连接数
	db.SetConnMaxLifetime(100)
	// 设置数据库最大的闲置连接数
	db.SetMaxIdleConns(10)
	// 验证连接
	if err = db.Ping(); nil != err {
		log.Println("Open Database Fail,Error:", err)
		checkErr(err)
	}
	return db
}

// Database 数据库字典实体
type Database struct {
	Name                    sql.NullString
	DefaultCharacterSetName sql.NullString
	DefaultCollationName    sql.NullString
	Tables                  []Table
}

// Table 数据库表字典实体
type Table struct {
	Name          sql.NullString
	Comment       sql.NullString
	CollationName sql.NullString
	CreateTime    sql.NullString
	Columns       []Column
}

// Column 数据库字段字典实体
type Column struct {
	Name             sql.NullString
	Type             sql.NullString
	Nullable         sql.NullString
	Primary          sql.NullString
	CharacterSetName sql.NullString
	CollationName    sql.NullString
	Default          sql.NullString
	Extra            sql.NullString
	Comment          sql.NullString
}

// 抛出异常
func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}
