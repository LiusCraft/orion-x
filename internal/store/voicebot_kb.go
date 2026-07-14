package store

import "gorm.io/gorm"

// VoicebotKB 智能体与知识库的多对多绑定关系
type VoicebotKB struct {
	VoicebotID string `gorm:"primaryKey;type:varchar(36);not null" json:"voicebot_id"`
	KBID       string `gorm:"primaryKey;type:varchar(36);not null" json:"kb_id"`
}

func (VoicebotKB) TableName() string { return "voicebot_kb_bindings" }

// VoicebotKBStore 智能体-知识库绑定
type VoicebotKBStore struct{ db *gorm.DB }

func NewVoicebotKBStore(db *gorm.DB) *VoicebotKBStore {
	return &VoicebotKBStore{db: db}
}

func (s *VoicebotKBStore) Bind(voicebotID, kbID string) error {
	return s.db.Create(&VoicebotKB{VoicebotID: voicebotID, KBID: kbID}).Error
}

func (s *VoicebotKBStore) Unbind(voicebotID, kbID string) error {
	return s.db.Delete(&VoicebotKB{}, "voicebot_id = ? AND kb_id = ?", voicebotID, kbID).Error
}

// ListKBIDsByVoicebot 返回智能体绑定的所有知识库 ID
func (s *VoicebotKBStore) ListKBIDsByVoicebot(voicebotID string) ([]string, error) {
	var bindings []VoicebotKB
	if err := s.db.Where("voicebot_id = ?", voicebotID).Find(&bindings).Error; err != nil {
		return nil, err
	}
	ids := make([]string, len(bindings))
	for i, b := range bindings {
		ids[i] = b.KBID
	}
	return ids, nil
}

// ListVoicebotIDsByKB 返回绑定了该知识库的所有智能体 ID
func (s *VoicebotKBStore) ListVoicebotIDsByKB(kbID string) ([]string, error) {
	var bindings []VoicebotKB
	if err := s.db.Where("kb_id = ?", kbID).Find(&bindings).Error; err != nil {
		return nil, err
	}
	ids := make([]string, len(bindings))
	for i, b := range bindings {
		ids[i] = b.VoicebotID
	}
	return ids, nil
}

// DeleteByVoicebot 解除智能体的所有绑定
func (s *VoicebotKBStore) DeleteByVoicebot(voicebotID string) error {
	return s.db.Delete(&VoicebotKB{}, "voicebot_id = ?", voicebotID).Error
}

// DeleteByKB 解除知识库的所有绑定
func (s *VoicebotKBStore) DeleteByKB(kbID string) error {
	return s.db.Delete(&VoicebotKB{}, "kb_id = ?", kbID).Error
}
