package models

import "time"

type UserOffense struct {
	ID        int
	CreatedAt time.Time
	UserID    int
	Type      UserOffenseType
}

const OffenseMessage = `您因为多次违规，账号被锁定，如有意见请发送邮件至 txsun19@fudan.edu.cn`

type UserOffenseType = int

const (
	UserOffensePrompt UserOffenseType = iota + 1
	UserOffenseMoss
)

func (user *User) AddUserOffense(offenseType UserOffenseType) (bool, error) {
	var offense = UserOffense{
		UserID: user.ID,
		Type:   offenseType,
	}
	err := DB.Create(&offense).Error
	if err != nil {
		return false, err
	}
	return user.CheckUserOffense()
}

func (user *User) CheckUserOffense() (bool, error) {
	if user.Banned {
		return true, nil
	}
	var (
		count int64
		err   error
	)
	err = DB.Model(&UserOffense{}).
		Where("created_at between ? and ? and type = ?", time.Now().Add(-5*time.Minute), time.Now(), UserOffensePrompt).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	if count >= 3 {
		user.Banned = true
		err = DB.Save(user).Error
		if err != nil {
			return false, err
		}
		return true, err
	}

	err = DB.Model(&UserOffense{}).
		Where("created_at between ? and ? and type = ?", time.Now().Add(-5*time.Minute), time.Now(), UserOffenseMoss).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	if count >= 10 {
		user.Banned = true
		err = DB.Save(user).Error
		if err != nil {
			return false, err
		}
		return true, err
	}
	return false, err
}
