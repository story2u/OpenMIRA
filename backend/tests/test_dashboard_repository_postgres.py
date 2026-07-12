"""Dashboard 仓储的筛选/排序/分页/隔离矩阵（需要 PostgreSQL，因 JSONB 运算符）。

无 SUBSCRIPTION_TEST_DATABASE_URL 时整文件跳过，与现有 Postgres 测试一致。
"""

import os
from collections.abc import AsyncIterator
from datetime import UTC, datetime, timedelta
from uuid import uuid4

import pytest
from sqlalchemy import delete
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine
from sqlmodel.ext.asyncio.session import AsyncSession

from app.domain.enums import FrontendOpportunityStatus, IMChannel, OpportunityStatus, Priority
from app.infrastructure.db.models import Opportunity, User
from app.infrastructure.db.repositories import OpportunityRepository

TEST_DATABASE_URL = os.getenv("SUBSCRIPTION_TEST_DATABASE_URL")
pytestmark = pytest.mark.skipif(
    not TEST_DATABASE_URL,
    reason="SUBSCRIPTION_TEST_DATABASE_URL is required for PostgreSQL dashboard tests",
)


async def _make_opp(session: AsyncSession, owner_id, **overrides) -> Opportunity:
    defaults = dict(
        owner_user_id=owner_id,
        channel=IMChannel.TELEGRAM,
        conversation_id=f"conv-{uuid4()}",
        title="opp",
        summary="opp summary",
        source_type="group",
        status=OpportunityStatus.PENDING_HUMAN,
        priority=Priority.NORMAL,
        trust_score=70,
        confidence=0.5,
        sop_stage="detected",
        matched_keywords=[],
        attention_required=False,
        last_message_at=datetime.now(UTC),
    )
    defaults.update(overrides)
    opp = Opportunity(**defaults)
    session.add(opp)
    await session.commit()
    await session.refresh(opp)
    return opp


@pytest.fixture
async def subject() -> AsyncIterator[tuple[async_sessionmaker[AsyncSession], User, User]]:
    assert TEST_DATABASE_URL
    engine = create_async_engine(TEST_DATABASE_URL)
    session_factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)
    owner = User(email=f"dash-owner-{os.urandom(6).hex()}@example.test")
    other = User(email=f"dash-other-{os.urandom(6).hex()}@example.test")
    async with session_factory() as session:
        session.add(owner)
        session.add(other)
        await session.commit()
    yield session_factory, owner, other
    async with session_factory() as session:
        await session.exec(delete(Opportunity).where(Opportunity.owner_user_id == owner.id))
        await session.exec(delete(Opportunity).where(Opportunity.owner_user_id == other.id))
        await session.exec(delete(User).where(User.id.in_([owner.id, other.id])))
        await session.commit()
    await engine.dispose()


async def test_owner_isolation(subject) -> None:
    factory, owner, other = subject
    async with factory() as session:
        await _make_opp(session, owner.id)
        await _make_opp(session, other.id)
        repo = OpportunityRepository(session)
        items, total = await repo.dashboard(owner_user_id=owner.id, limit=20, offset=0)
        assert total == 1
        assert all(o.owner_user_id == owner.id for o in items)


async def test_status_and_platform_filter(subject) -> None:
    factory, owner, _ = subject
    async with factory() as session:
        await _make_opp(session, owner.id, status=OpportunityStatus.PENDING_HUMAN, channel=IMChannel.TELEGRAM)
        await _make_opp(session, owner.id, status=OpportunityStatus.REPLIED, channel=IMChannel.WECOM)
        repo = OpportunityRepository(session)
        _, pending_total = await repo.dashboard(
            owner_user_id=owner.id, frontend_status=FrontendOpportunityStatus.PENDING
        )
        _, wecom_total = await repo.dashboard(owner_user_id=owner.id, channel=IMChannel.WECOM)
        assert pending_total == 1
        assert wecom_total == 1


async def test_source_type_filter(subject) -> None:
    factory, owner, _ = subject
    async with factory() as session:
        await _make_opp(session, owner.id, source_type="group")
        await _make_opp(session, owner.id, source_type="private")
        repo = OpportunityRepository(session)
        _, group_total = await repo.dashboard(owner_user_id=owner.id, source_type="group")
        assert group_total == 1


async def test_time_range_filter(subject) -> None:
    factory, owner, _ = subject
    async with factory() as session:
        old = await _make_opp(session, owner.id)
        old.created_at = datetime.now(UTC) - timedelta(days=10)
        session.add(old)
        await session.commit()
        await _make_opp(session, owner.id)
        repo = OpportunityRepository(session)
        _, recent_total = await repo.dashboard(
            owner_user_id=owner.id, created_from=datetime.now(UTC) - timedelta(days=3)
        )
        assert recent_total == 1


async def test_trust_range_filter(subject) -> None:
    factory, owner, _ = subject
    async with factory() as session:
        await _make_opp(session, owner.id, trust_score=90)  # trusted
        await _make_opp(session, owner.id, trust_score=30)  # risky
        repo = OpportunityRepository(session)
        _, trusted_total = await repo.dashboard(owner_user_id=owner.id, trust_ranges=[(80, 100)])
        _, multi_total = await repo.dashboard(
            owner_user_id=owner.id, trust_ranges=[(80, 100), (0, 39)]
        )
        assert trusted_total == 1
        assert multi_total == 2


async def test_sop_stage_and_keyword_filter(subject) -> None:
    factory, owner, _ = subject
    async with factory() as session:
        await _make_opp(session, owner.id, sop_stage="verified", matched_keywords=["报价", "采购"])
        await _make_opp(session, owner.id, sop_stage="detected", matched_keywords=["招聘"])
        repo = OpportunityRepository(session)
        _, verified_total = await repo.dashboard(owner_user_id=owner.id, sop_stages=["verified"])
        _, kw_total = await repo.dashboard(owner_user_id=owner.id, keywords=["报价"])
        _, kw_any_total = await repo.dashboard(owner_user_id=owner.id, keywords=["报价", "招聘"])
        assert verified_total == 1
        assert kw_total == 1
        assert kw_any_total == 2


async def test_sorting(subject) -> None:
    factory, owner, _ = subject
    async with factory() as session:
        a = await _make_opp(session, owner.id, confidence=0.9, trust_score=50)
        b = await _make_opp(session, owner.id, confidence=0.2, trust_score=95)
        a.created_at = datetime.now(UTC) - timedelta(hours=2)
        b.created_at = datetime.now(UTC)
        session.add(a)
        session.add(b)
        await session.commit()
        repo = OpportunityRepository(session)
        newest, _ = await repo.dashboard(owner_user_id=owner.id, sort="newest")
        oldest, _ = await repo.dashboard(owner_user_id=owner.id, sort="oldest")
        by_conf, _ = await repo.dashboard(owner_user_id=owner.id, sort="confidence")
        by_trust, _ = await repo.dashboard(owner_user_id=owner.id, sort="trust")
        assert newest[0].id == b.id
        assert oldest[0].id == a.id
        assert by_conf[0].id == a.id
        assert by_trust[0].id == b.id


async def test_pending_count_ignores_filters(subject) -> None:
    factory, owner, _ = subject
    async with factory() as session:
        await _make_opp(session, owner.id, status=OpportunityStatus.PENDING_HUMAN, channel=IMChannel.TELEGRAM)
        await _make_opp(session, owner.id, status=OpportunityStatus.PENDING_HUMAN, channel=IMChannel.WECOM)
        await _make_opp(session, owner.id, status=OpportunityStatus.REPLIED)
        repo = OpportunityRepository(session)
        # 即便筛选到只有 1 条 wecom，pendingCount 仍应是全部 2 条待处理
        _, wecom_total = await repo.dashboard(owner_user_id=owner.id, channel=IMChannel.WECOM)
        pending = await repo.count_pending(owner.id)
        assert wecom_total == 1
        assert pending == 2


async def test_attention_items(subject) -> None:
    factory, owner, _ = subject
    async with factory() as session:
        await _make_opp(session, owner.id, attention_required=True, status=OpportunityStatus.PENDING_HUMAN)
        # attention 但已回复 -> 不计入
        await _make_opp(session, owner.id, attention_required=True, status=OpportunityStatus.REPLIED)
        repo = OpportunityRepository(session)
        attention = await repo.list_attention(owner.id)
        assert len(attention) == 1


async def test_pagination_and_total(subject) -> None:
    factory, owner, _ = subject
    async with factory() as session:
        for _ in range(5):
            await _make_opp(session, owner.id)
        repo = OpportunityRepository(session)
        page1, total = await repo.dashboard(owner_user_id=owner.id, limit=2, offset=0)
        page3, _ = await repo.dashboard(owner_user_id=owner.id, limit=2, offset=4)
        assert total == 5
        assert len(page1) == 2
        assert len(page3) == 1


async def test_keyword_options_from_real_data(subject) -> None:
    factory, owner, other = subject
    async with factory() as session:
        await _make_opp(session, owner.id, matched_keywords=["报价", "采购"])
        await _make_opp(session, owner.id, matched_keywords=["报价"])
        await _make_opp(session, other.id, matched_keywords=["泄漏关键词"])
        repo = OpportunityRepository(session)
        options = await repo.keyword_options(owner.id)
        assert set(options) == {"报价", "采购"}
        assert "泄漏关键词" not in options


async def test_empty_result(subject) -> None:
    factory, owner, _ = subject
    async with factory() as session:
        repo = OpportunityRepository(session)
        items, total = await repo.dashboard(owner_user_id=owner.id)
        assert items == []
        assert total == 0
