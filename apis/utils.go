package apis

import (
	. "MOSS_backend/models"
	. "MOSS_backend/utils"
)

func infer(input string, records Records) (output string, duration float64, err error) {
	return InferMosec(input, records.ToRecordModel())
	//timeNow := time.Now().Unix()
	//if timeNow%2 == 0 {
	//	// use triton
	//	// get all params to infer server
	//	var params Params
	//	err = DB.Find(&params).Error
	//	if err != nil {
	//		return "", 0, err
	//	}
	//
	//	return InferTriton(input, records.ToRecordModel(), params)
	//} else {
	//	// use Mosec
	//
	//}
}
