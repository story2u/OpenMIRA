from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass
from datetime import datetime
from uuid import uuid4

from app.domain.enums import (
    JobWorkMode,
    OpportunityStatus,
    OpportunityType,
)
from app.domain.job_models import JobAgentAnalysis
from app.domain.services.job_discovery import normalize_text
from app.infrastructure.db.models import (
    JobOpportunityDetail,
    JobOpportunitySource,
    Message,
    Opportunity,
    SourceFunctionalProfile,
)
from app.infrastructure.db.repositories import JobOpportunityRepository

IMPORTANT_EVIDENCE = {
    "job_title": "job_title",
    "company_name": "company_name",
    "location_text": "location",
    "application_url": "application_url",
    "age_requirement_text": "age_requirement",
}


@dataclass(frozen=True, slots=True)
class PersistedJobResult:
    opportunity: Opportunity | None
    rejected_reason: str | None = None


class PersistJobOpportunityUseCase:
    def __init__(self, repository: JobOpportunityRepository) -> None:
        self.repository = repository

    async def execute(
        self,
        *,
        message: Message,
        analysis: JobAgentAnalysis,
        source_profile: SourceFunctionalProfile,
        existing_opportunity: Opportunity | None,
    ) -> PersistedJobResult:
        if not message.owner_user_id or not analysis.is_formal_job() or not analysis.job:
            return PersistedJobResult(None, "not a formal job posting")
        if analysis.extraction_confidence < 0.5:
            return PersistedJobResult(None, "job extraction confidence below review threshold")
        missing_evidence = self._missing_evidence(analysis)
        if missing_evidence:
            return PersistedJobResult(None, f"missing evidence: {', '.join(missing_evidence)}")

        job = analysis.job
        source_message_url = self._source_message_url(message)
        normalized_title = normalize_text(job.normalized_job_title or job.job_title)
        fingerprint = self._fingerprint(
            application_url=job.application_url,
            company_name=job.company_name,
            title=normalized_title,
            location=job.location_text,
            text=message.text or "",
        )
        duplicate = await self.repository.find_duplicate(
            owner_user_id=message.owner_user_id,
            application_url=self._normalize_url(job.application_url),
            company_name=job.company_name,
            normalized_job_title=normalized_title,
            city=job.city,
            content_fingerprint=fingerprint,
            exclude_opportunity_id=existing_opportunity.id if existing_opportunity else None,
        )
        opportunity = existing_opportunity or Opportunity(
            id=uuid4(),
            owner_user_id=message.owner_user_id,
            opportunity_type=OpportunityType.JOB,
            channel=message.channel,
            conversation_id=message.conversation_id,
            customer_external_id=message.sender_external_id,
            contact_name=message.sender_display_name or "招聘信息发布者",
            source_type=message.source_type,
            group_name=message.group_name,
            source_message_id=message.id,
            title=job.job_title,
            summary=job.requirements_summary,
            confidence=analysis.extraction_confidence,
            detection_reason="pi agent job discovery",
            status=OpportunityStatus.PENDING_HUMAN,
            last_message_preview=(message.text or "")[:2000],
            last_message_at=message.sent_at,
        )
        compliance_flags = list(dict.fromkeys(analysis.compliance_flags))
        if job.age_requirement_text:
            compliance_flags.append("potentialAgeDiscrimination")
        if self._protected_restriction_present(message.text or ""):
            compliance_flags.append("potentialProtectedAttributeRestriction")
        deadline = self._parse_datetime(job.application_deadline)
        detail = JobOpportunityDetail(
            opportunity_id=opportunity.id,
            source_channel=message.channel,
            source_chat_id=message.conversation_id,
            source_chat_name=message.group_name,
            source_message_id=message.external_message_id,
            source_message_url=source_message_url,
            source_author_name=message.sender_display_name,
            source_author_username=self._sender_username(message),
            source_reliability_score=source_profile.reliability_score,
            posted_at=message.sent_at,
            job_title=job.job_title,
            normalized_job_title=normalized_title,
            company_name=normalize_text(job.company_name) if job.company_name else None,
            department=job.department,
            company_industry=job.company_industry,
            company_stage=job.company_stage,
            location_text=job.location_text,
            country_code=job.country_code.upper() if job.country_code else None,
            city=normalize_text(job.city) if job.city else None,
            timezone=job.timezone,
            work_mode=job.work_mode,
            employment_type=job.employment_type,
            seniority=job.seniority,
            salary_raw=job.salary.raw,
            salary_min=job.salary.minimum,
            salary_max=job.salary.maximum,
            salary_currency=job.salary.currency.upper() if job.salary.currency else None,
            salary_period=job.salary.period,
            salary_negotiable=job.salary.negotiable,
            equity_mentioned=job.equity_mentioned,
            requirements_summary=job.requirements_summary,
            required_skills=job.required_skills,
            preferred_skills=job.preferred_skills,
            minimum_years_experience=job.minimum_years_experience,
            maximum_years_experience=job.maximum_years_experience,
            degree_required=job.degree_required,
            degree_level=job.degree_level,
            degree_field=job.degree_field,
            english_level=job.english_level,
            other_language_requirements=job.other_language_requirements,
            visa_sponsorship=job.visa_sponsorship,
            work_authorization_text=job.work_authorization_text,
            relocation_support=job.relocation_support,
            age_requirement_text=job.age_requirement_text,
            age_requirement_present=bool(job.age_requirement_text),
            application_deadline=deadline,
            application_url=self._normalize_url(job.application_url),
            contact_methods=[item.model_dump() for item in job.contact_methods],
            compliance_flags=list(dict.fromkeys(compliance_flags)),
            extraction_confidence=analysis.extraction_confidence,
            missing_fields=analysis.missing_fields,
            field_evidence=analysis.field_evidence,
            raw_excerpt=(message.text or "")[:4000],
            content_fingerprint=fingerprint,
        )
        source = JobOpportunitySource(
            opportunity_id=opportunity.id,
            message_id=message.id,
            owner_user_id=message.owner_user_id,
            source_message_url=source_message_url,
            source_chat_name=message.group_name,
            source_author_name=message.sender_display_name,
            posted_at=message.sent_at,
            source_reliability_score=source_profile.reliability_score,
        )
        persisted = await self.repository.save_projection(
            opportunity=opportunity,
            detail=detail,
            source=source,
            message=message,
            canonical=duplicate,
        )
        return PersistedJobResult(persisted)

    @staticmethod
    def _missing_evidence(analysis: JobAgentAnalysis) -> list[str]:
        assert analysis.job
        missing = ["job_title"] if not analysis.field_evidence.get("job_title") else []
        for attribute, evidence_key in IMPORTANT_EVIDENCE.items():
            if attribute == "job_title":
                continue
            if getattr(analysis.job, attribute) is not None and not analysis.field_evidence.get(
                evidence_key
            ):
                missing.append(attribute)
        if analysis.job.salary.raw and not analysis.field_evidence.get("salary"):
            missing.append("salary")
        if analysis.job.work_mode != JobWorkMode.UNKNOWN and not analysis.field_evidence.get(
            "work_mode"
        ):
            missing.append("work_mode")
        return missing

    @staticmethod
    def _fingerprint(
        *,
        application_url: str | None,
        company_name: str | None,
        title: str,
        location: str | None,
        text: str,
    ) -> str:
        stable = "|".join(
            [
                PersistJobOpportunityUseCase._normalize_url(application_url) or "",
                normalize_text(company_name or ""),
                title,
                normalize_text(location or ""),
                normalize_text(text)[:2000],
            ]
        )
        return hashlib.sha256(stable.encode()).hexdigest()

    @staticmethod
    def _normalize_url(value: str | None) -> str | None:
        return value.strip()[:2000] if value and value.startswith(("http://", "https://")) else None

    @staticmethod
    def _parse_datetime(value: str | None) -> datetime | None:
        if not value:
            return None
        try:
            parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
            return parsed if parsed.tzinfo else None
        except ValueError:
            return None

    @staticmethod
    def _source_message_url(message: Message) -> str | None:
        if message.channel.value != "telegram":
            return None
        payload = message.raw_payload if isinstance(message.raw_payload, dict) else {}
        provider_message = (
            payload.get("message")
            or payload.get("channel_post")
            or payload.get("business_message")
            or payload
        )
        chat = provider_message.get("chat") if isinstance(provider_message, dict) else None
        username = (
            chat.get("username") if isinstance(chat, dict) else payload.get("source_username")
        )
        provider_id = (
            provider_message.get("message_id")
            if isinstance(provider_message, dict)
            else payload.get("message_id")
        )
        if not username or not provider_id:
            return None
        return f"https://t.me/{str(username).lstrip('@')}/{provider_id}"

    @staticmethod
    def _sender_username(message: Message) -> str | None:
        payload = message.raw_payload if isinstance(message.raw_payload, dict) else {}
        provider_message = payload.get("message") or payload.get("channel_post") or payload
        sender = provider_message.get("from") if isinstance(provider_message, dict) else None
        return (
            str(sender.get("username"))[:255]
            if isinstance(sender, dict) and sender.get("username")
            else None
        )

    @staticmethod
    def _protected_restriction_present(text: str) -> bool:
        return bool(re.search(r"(限男|限女|男性优先|女性优先|未婚|已婚|民族要求|宗教要求)", text))
