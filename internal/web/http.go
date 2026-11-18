package web

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"biaoge-bi/internal/model"
)

const CtxUserKey = "bi_user"

type APIResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type PageData struct {
	Items    any   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, APIResponse{Code: 0, Message: "ok", Data: data})
}

func Fail(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, APIResponse{Code: status, Message: message})
}

func BadRequest(c *gin.Context, msg string)   { Fail(c, http.StatusBadRequest, msg) }
func Unauthorized(c *gin.Context, msg string) { Fail(c, http.StatusUnauthorized, msg) }
func Forbidden(c *gin.Context, msg string)    { Fail(c, http.StatusForbidden, msg) }
func NotFound(c *gin.Context, msg string)     { Fail(c, http.StatusNotFound, msg) }
func Conflict(c *gin.Context, msg string)     { Fail(c, http.StatusConflict, msg) }
func Unprocessable(c *gin.Context, msg string) { Fail(c, http.StatusUnprocessableEntity, msg) }
func ServerError(c *gin.Context, msg string)  { Fail(c, http.StatusInternalServerError, msg) }

func Page(c *gin.Context, items any, total int64, page, pageSize int) {
	OK(c, PageData{Items: items, Total: total, Page: page, PageSize: pageSize})
}

func SetUser(c *gin.Context, u *model.User) { c.Set(CtxUserKey, u) }

func CurrentUser(c *gin.Context) *model.User {
	v, ok := c.Get(CtxUserKey)
	if !ok {
		return nil
	}
	u, _ := v.(*model.User)
	return u
}
