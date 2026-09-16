package service

import (
	"context"
	"encoding/json"
	"testing"

	"driftbottle/internal/domain"
)

type targetAI struct{}

func (targetAI) Run(_ context.Context, task string, _ any, out any) error {
	if task != "target" {
		return domain.Fail(503, "AI_UNAVAILABLE", "test")
	}
	v := map[string]any{"requiredExperiences": []string{"照顾长期失眠家人的经历"}}
	return json.Unmarshal([]byte(domain.JSON(v)), out)
}

func TestMatchTargetUsesBackgroundAI(t *testing.T) {
	s := &Service{AI: targetAI{}}
	target := s.matchTarget(context.Background(), domain.Bottle{
		ID:   "b1",
		Raw:  "家里有人长期失眠，我不知道怎么陪伴",
		Hint: "想听有类似经历的人说说",
		Target: domain.Target{Required: []string{
			"亲身经历过与这段描述相似的处境",
		}},
	}, nil)
	if len(target.Required) != 1 || target.Required[0] != "照顾长期失眠家人的经历" {
		t.Fatalf("background target was not used: %+v", target)
	}
}
