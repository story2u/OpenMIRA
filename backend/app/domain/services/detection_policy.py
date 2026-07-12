import re

from app.domain.enums import Priority, RuleType
from app.domain.ports import (
    ConversationTurn,
    DetectionResult,
    DetectionRule,
    OpportunityAIClassifier,
    OpportunityClassificationRequest,
)

HIGH_INTENT_WORDS = (
    "报价",
    "价格",
    "采购",
    "私有化",
    "部署",
    "企业版",
    "API",
    "合同",
    "续约",
    "折扣",
    "试用",
    "demo",
    "pricing",
    "quote",
    "enterprise",
    "trial",
)

JOB_INTENT_WORDS = (
    "招聘",
    "招人",
    "岗位",
    "职位",
    "内推",
    "简历",
    "投递",
    "薪资",
    "薪酬",
    "远程",
    "全职",
    "兼职",
    "实习",
    "外包",
    "工程师",
    "开发",
    "后端",
    "前端",
    "算法",
    "产品经理",
    "运营",
    "设计师",
    "hiring",
    "recruiting",
    "job",
    "jobs",
    "remote",
    "full-time",
    "part-time",
    "resume",
    "cv",
)

CONTACT_SIGNAL_RE = re.compile(
    r"(@[A-Za-z0-9_]{4,}|[\w.+-]+@[\w-]+(?:\.[\w-]+)+|(?:微信|VX|vx|TG|Telegram|联系))"
)
SALARY_SIGNAL_RE = re.compile(r"(\d{1,3}\s*[kK]|[\d.]+\s*[万wW]|薪资|薪酬|预算|base)")


class OpportunityDetector:
    def __init__(self, ai_classifier: OpportunityAIClassifier | None = None) -> None:
        self.ai_classifier = ai_classifier

    async def detect(
        self,
        text: str,
        rules: list[DetectionRule],
        *,
        conversation: list[ConversationTurn] | None = None,
        source_type: str = "private",
        group_name: str | None = None,
    ) -> DetectionResult:
        normalized = text.strip()
        if not normalized:
            return DetectionResult(is_opportunity=False)

        score = 0.0
        reasons: list[str] = []
        matched_keywords: list[str] = []

        for rule in sorted(rules, key=lambda item: item.priority):
            matched = self._match_rule(rule, normalized)
            if not matched:
                continue

            score = min(1.0, score + rule.score)
            reasons.append(f"{rule.name}:{matched}")
            if rule.rule_type in {RuleType.KEYWORD, RuleType.AI_HINT}:
                matched_keywords.append(matched)

        for word in HIGH_INTENT_WORDS:
            if word.lower() in normalized.lower() and word not in matched_keywords:
                score = min(1.0, score + 0.12)
                matched_keywords.append(word)

        job_score, job_keywords = self._job_posting_score(normalized)
        if job_score > 0:
            score = min(1.0, score + job_score)
            for keyword in job_keywords:
                if keyword not in matched_keywords:
                    matched_keywords.append(keyword)
            reasons.append("recruiting_signal")

        if score >= 0.75:
            return self._build_positive_result(normalized, score, reasons, matched_keywords)

        rule_result = DetectionResult(
            is_opportunity=score >= 0.45,
            confidence=score,
            title=self._title(normalized) if score >= 0.45 else None,
            summary=normalized if score >= 0.45 else None,
            reason="; ".join(reasons) if reasons else None,
            matched_keywords=matched_keywords,
            priority=self._priority(score, matched_keywords),
        )
        if not self.ai_classifier:
            return rule_result

        request = OpportunityClassificationRequest(
            text=normalized,
            rule_score=score,
            matched_keywords=matched_keywords,
            ai_hints=self._ai_hints(rules),
            conversation=conversation or [],
            source_type=source_type,
            group_name=group_name,
        )
        try:
            ai_result = await self.ai_classifier.classify(request)
        except Exception:
            # Provider failures must not interrupt durable message ingestion. The caller still
            # receives the same deterministic rule result it would get with AI disabled.
            return rule_result
        if ai_result is None:
            return rule_result
        if ai_result.is_opportunity:
            # Expanding semantic recall must not also expand automatic-send authority.
            ai_result.requires_human_review = True
        if matched_keywords:
            ai_result.matched_keywords = sorted(
                {*ai_result.matched_keywords, *matched_keywords},
                key=lambda item: normalized.lower().find(item.lower())
                if item.lower() in normalized.lower()
                else 999,
            )
        return ai_result

    def _ai_hints(self, rules: list[DetectionRule]) -> list[str]:
        hints: list[str] = []
        for rule in sorted(rules, key=lambda item: item.priority):
            if rule.rule_type != RuleType.AI_HINT:
                continue
            for candidate in (item.strip() for item in rule.pattern.split(",")):
                normalized = candidate[:240]
                if normalized and normalized not in hints:
                    hints.append(normalized)
                if len(hints) >= 20:
                    return hints
        return hints

    def _match_rule(self, rule: DetectionRule, text: str) -> str | None:
        if rule.rule_type in {RuleType.KEYWORD, RuleType.AI_HINT}:
            candidates = [item.strip() for item in rule.pattern.split(",") if item.strip()]
            for candidate in candidates:
                if candidate.lower() in text.lower():
                    return candidate
            return None

        if rule.rule_type == RuleType.REGEX:
            match = re.search(rule.pattern, text, flags=re.IGNORECASE)
            return match.group(0) if match else None

        return None

    def _build_positive_result(
        self,
        text: str,
        score: float,
        reasons: list[str],
        matched_keywords: list[str],
    ) -> DetectionResult:
        return DetectionResult(
            is_opportunity=True,
            confidence=score,
            title=self._title(text),
            summary=text[:240],
            reason="; ".join(reasons) if reasons else "high intent keyword match",
            matched_keywords=matched_keywords[:8],
            priority=self._priority(score, matched_keywords),
        )

    def _priority(self, score: float, matched_keywords: list[str]) -> Priority:
        urgent_words = {"报价", "采购", "合同", "urgent", "quote", "招聘", "hiring"}
        if score >= 0.92 or urgent_words.intersection({item.lower() for item in matched_keywords}):
            return Priority.URGENT
        if score >= 0.8:
            return Priority.HIGH
        if score >= 0.55:
            return Priority.NORMAL
        return Priority.LOW

    def _title(self, text: str) -> str:
        return text[:42] + ("..." if len(text) > 42 else "")

    def _job_posting_score(self, text: str) -> tuple[float, list[str]]:
        lower_text = text.lower()
        matched = [word for word in JOB_INTENT_WORDS if word.lower() in lower_text]
        if not matched:
            return 0.0, []

        score = min(0.58, 0.18 + 0.1 * min(len(matched), 4))
        if SALARY_SIGNAL_RE.search(text):
            score += 0.12
            matched.append("薪资")
        if CONTACT_SIGNAL_RE.search(text):
            score += 0.12
            matched.append("联系方式")
        if re.search(r"(python|java|golang|go|react|node|fastapi|ai|llm)", lower_text):
            score += 0.08

        return min(score, 0.78), matched[:8]
