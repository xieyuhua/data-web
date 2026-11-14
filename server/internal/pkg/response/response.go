package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Body 统一响应结构。
// Code 为 0 表示成功，非 0 时与 HTTP 状态码保持一致；Data 为空时序列化会被省略。
type Body struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// OK 返回 200 成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Code: 0, Message: "ok", Data: data})
}

// Created 返回 201 创建成功响应。
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Body{Code: 0, Message: "created", Data: data})
}

// Fail 返回失败响应，并中断后续中间件/处理器。
func Fail(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, Body{Code: status, Message: message})
}

// BadRequest 返回 400 参数错误。
func BadRequest(c *gin.Context, message string) {
	Fail(c, http.StatusBadRequest, message)
}

// Unauthorized 返回 401 未认证。
func Unauthorized(c *gin.Context, message string) {
	Fail(c, http.StatusUnauthorized, message)
}

// Forbidden 返回 403 无权限。
func Forbidden(c *gin.Context, message string) {
	Fail(c, http.StatusForbidden, message)
}

// NotFound 返回 404 资源不存在。
func NotFound(c *gin.Context, message string) {
	Fail(c, http.StatusNotFound, message)
}

// ServerError 返回 500 服务端错误（携带原始错误信息便于排查）。
func ServerError(c *gin.Context, err error) {
	Fail(c, http.StatusInternalServerError, err.Error())
}
