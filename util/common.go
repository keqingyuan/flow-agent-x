package util

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"runtime"
	"strings"
)

const (
	CRLF          = "\r\n"
	UnixLineBreak = "\n"

	EmptyStr = ""

	OSWin   = "windows"
	OSLinux = "linux"
	OSMac   = "darwin"
)

func OS () string {
	if IsMac() {
		return "MAC"
	}

	if IsWindows() {
		return "WIN"
	}

	if IsLinux() {
		return "LINUX"
	}

	return "UNKNOWN"
}

func IsMac() bool {
	return runtime.GOOS == OSMac
}

func IsLinux() bool {
	return runtime.GOOS == OSLinux
}

func IsWindows() bool {
	return runtime.GOOS == OSWin
}

func HasError(err error) bool {
	return err != nil
}

// FailOnError exit program with err
func FailOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

// IsEmptyString to check input s is empty
func IsEmptyString(s string) bool {
	return s == ""
}

// IsPointerType to check the input v is pointer type
func IsPointerType(v interface{}) bool {
	return reflect.ValueOf(v).Kind() == reflect.Ptr
}

// GetType get type of pointer
func GetType(v interface{}) reflect.Type {
	if IsPointerType(v) {
		val := reflect.ValueOf(v)
		return val.Elem().Type()
	}

	return reflect.TypeOf(v)
}

func GetValue(v interface{}) reflect.Value {
	val := reflect.ValueOf(v)

	if val.Kind() == reflect.Ptr {
		return val.Elem()
	}

	return val
}

// ParseString parse string which include system env variable
func ParseString(src string) string {
	return parseVariablesFrom(src, os.Getenv)
}

func ParseStringWithSource(src string, source map[string]string) string {
	return parseVariablesFrom(src, func(env string) string {
		return source[env]
	})
}

// replace ${VAR} with actual variable value
func parseVariablesFrom(src string, getVariable func(string)string) string {
	if IsEmptyString(src) {
		return src
	}

	for i := 0; i < len(src); i++ {
		if src[i] != '$' {
			continue
		}

		// left bracket index
		lIndex := i + 1
		if src[lIndex] != '{' {
			continue
		}

		// find right bracket index
		for rIndex := lIndex + 1; rIndex < len(src); rIndex++ {
			if src[rIndex] != '}' {
				continue
			}

			env := src[lIndex+1 : rIndex]
			val := getVariable(env)

			// do not replace if no value found
			if IsEmptyString(val) {
				break
			}

			src = strings.Replace(src, fmt.Sprintf("${%s}", env), val, -1)
			i = rIndex
			break
		}
	}

	return src
}

func GetEnv(env, def string) string {
	val, ok := os.LookupEnv(env)
	if ok {
		return val
	}
	return def
}

func ByteToMB(bytes uint64) uint64 {
	return (bytes / 1024) / 1024
}

func PanicIfErr(err error) {
	if err != nil {
		panic(err)
	}
}
