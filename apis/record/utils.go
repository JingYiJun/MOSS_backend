package record

import (
	. "MOSS_backend/utils"
	"errors"
	"regexp"
)

// regexps
var (
	endContentRegexp              = regexp.MustCompile(`<[es]o\w>`)
	mossSpecialTokenRegexp        = regexp.MustCompile(`<eo[tcrmh]>`)
	innerThoughtsRegexp           = regexp.MustCompile(`<\|Inner Thoughts\|>:([\s\S]+?)(<eo\w>)`)
	commandsRegexp                = regexp.MustCompile(`<\|Commands\|>:([\s\S]+?)(<eo\w>)`)
	resultsRegexp                 = regexp.MustCompile(`<\|Results\|>:[\s\S]+?<eor>`) // not greedy
	mossRegexp                    = regexp.MustCompile(`<\|MOSS\|>:([\s\S]+?)(<eo\w>)`)
	secondGenerationsFormatRegexp = regexp.MustCompile(`^<\|MOSS\|>:[\s\S]+?<eo\w>$`)
	firstGenerationsFormatRegexp  = regexp.MustCompile(`^<\|Inner Thoughts\|>:[\s\S]+?<eo\w>\n *?<\|Commands\|>:[\s\S]+?<eo\w>$`)
	funcCallRegexp				  = regexp.MustCompile(`(?s)func_call\s*(.*?)<\|end_of_func_call\|>`)
)

//var maxLengthExceededError = BadRequest("The maximum context length is exceeded").WithMessageType(MaxLength)

// error messages
var (
	userRequestingError            = BadRequest("上一次请求还未结束，请稍后再试。User requesting, please wait and try again")
	maxInputExceededError          = BadRequest("单次输入限长为 2048 字符。Input no more than 2048 characters").WithMessageType(MaxLength)
	maxInputExceededFromInferError = BadRequest("单次输入超长，请减少字数并重试。Input max length exceeded, please reduce length and try again").WithMessageType(MaxLength)
	unknownError                   = InternalServerError("未知错误，请刷新或等待一分钟后再试。Unknown error, please refresh or wait a minute and try again")
	ErrSensitive                   = errors.New("sensitive")
	interruptError                 = NoStatus("client interrupt")
)
