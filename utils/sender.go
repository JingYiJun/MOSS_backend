package utils

import (
	"MOSS_backend/config"
	"fmt"
	unisms "github.com/apistd/uni-go-sdk/sms"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/regions"
	ses "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ses/v20201002"
	"go.uber.org/zap"
)

func SendCodeEmail(code, receiver string) error {
	credential := common.NewCredential(
		config.Config.TencentSecretID,
		config.Config.TencentSecretKey,
	)
	// 实例化一个client选项，可选的，没有特殊需求可以跳过
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "ses.tencentcloudapi.com"
	// 实例化要请求产品的client对象,clientProfile是可选的
	client, err := ses.NewClient(credential, regions.HongKong, cpf)
	if err != nil {
		return err
	}

	// 实例化一个请求对象,每个接口都会对应一个request对象
	request := ses.NewSendEmailRequest()

	request.FromEmailAddress = common.StringPtr(config.Config.EmailUrl)
	request.Destination = common.StringPtrs([]string{receiver})
	request.Template = &ses.Template{
		TemplateID:   common.Uint64Ptr(config.Config.TencentTemplateID),
		TemplateData: common.StringPtr(fmt.Sprintf("{\"code\": \"%s\"}", code)),
	}
	request.Subject = common.StringPtr("[MOSS] Verification Code")
	request.TriggerType = common.Uint64Ptr(1)

	// 返回的resp是一个SendEmailResponse的实例，与请求对象对应
	resp, err := client.SendEmail(request)
	if err != nil {
		return err
	}
	Logger.Info("SendEmailResponse", zap.String("Response", resp.ToJsonString()))
	return err
}

func SendCodeMessage(code, phone string) error {
	// 初始化
	client := unisms.NewClient(config.Config.UniAccessID) // 若使用简易验签模式仅传入第一个参数即可

	// 构建信息
	message := unisms.BuildMessage()
	message.SetTo(phone)
	message.SetSignature(config.Config.UniSignature)
	message.SetTemplateId(config.Config.UniTemplateID)
	message.SetTemplateData(map[string]string{"code": code}) // 设置自定义参数 (变量短信)

	// 发送短信
	_, err := client.Send(message)
	return err
}
