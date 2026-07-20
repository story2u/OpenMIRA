"""详情与消息读取端点：owner 隔离、兼容上限和分页元数据。"""

from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

from fastapi import FastAPI
from fastapi.testclient import TestClient

from app.api.deps import get_message_repo, get_opportunity_repo, require_user
from app.api.v1.routes import messages, opportunities
from app.domain.enums import IMChannel, MessageDirection, MessageSource, OpportunityType
from app.infrastructure.db.models import Message, Opportunity, User


class FakeOpportunityRepo:
    def __init__(self, opportunity: Opportunity | None) -> None:
        self.opportunity = opportunity

    async def get(self, opportunity_id: UUID) -> Opportunity | None:
        if self.opportunity and self.opportunity.id == opportunity_id:
            return self.opportunity
        return None


class FakeMessageRepo:
    def __init__(self, items: list[Message]) -> None:
        self.items = items
        self.calls: list[tuple[int, int]] = []

    async def list_by_opportunity(
        self,
        opportunity_id: UUID,
        *,
        limit: int,
        offset: int,
    ) -> list[Message]:
        self.calls.append((limit, offset))
        matching = [item for item in self.items if item.opportunity_id == opportunity_id]
        return matching[offset : offset + limit]

    async def list_recent_by_opportunity(
        self,
        opportunity_id: UUID,
        *,
        limit: int,
    ) -> list[Message]:
        self.calls.append((limit, -1))
        matching = [item for item in self.items if item.opportunity_id == opportunity_id]
        return matching[-limit:]

    async def count_by_opportunity(self, opportunity_id: UUID) -> int:
        return sum(item.opportunity_id == opportunity_id for item in self.items)


def make_opportunity(
    owner_id: UUID,
    opportunity_type: OpportunityType = OpportunityType.BUSINESS,
) -> Opportunity:
    return Opportunity(
        id=uuid4(),
        owner_user_id=owner_id,
        opportunity_type=opportunity_type,
        channel=IMChannel.TELEGRAM,
        conversation_id="fixture-conversation",
        title="采购咨询",
        summary="希望了解企业方案",
    )


def make_messages(opportunity_id: UUID, count: int) -> list[Message]:
    start = datetime(2026, 7, 17, 1, 0, tzinfo=UTC)
    return [
        Message(
            id=uuid4(),
            channel=IMChannel.TELEGRAM,
            external_message_id=f"message-{index}",
            conversation_id="fixture-conversation",
            sender_display_name="联系人" if index % 2 == 0 else "商机助手",
            direction=(
                MessageDirection.INCOMING if index % 2 == 0 else MessageDirection.OUTGOING
            ),
            source=MessageSource.HUMAN,
            text=f"消息 {index}",
            opportunity_id=opportunity_id,
            sent_at=start + timedelta(minutes=index),
        )
        for index in range(count)
    ]


def build_client(
    user: User,
    opportunity_repo: FakeOpportunityRepo,
    message_repo: FakeMessageRepo,
) -> TestClient:
    app = FastAPI()
    app.include_router(messages.router, prefix="/messages")
    app.include_router(opportunities.router, prefix="/opportunities")
    app.dependency_overrides[require_user] = lambda: user
    app.dependency_overrides[get_opportunity_repo] = lambda: opportunity_repo
    app.dependency_overrides[get_message_repo] = lambda: message_repo
    return TestClient(app)


def test_detail_returns_owned_resource_and_hides_foreign_resource() -> None:
    owner = User(id=uuid4(), email="owner@example.test")
    opportunity = make_opportunity(owner.id)
    repo = FakeOpportunityRepo(opportunity)
    client = build_client(owner, repo, FakeMessageRepo([]))

    response = client.get(f"/opportunities/{opportunity.id}")

    assert response.status_code == 200
    assert response.json()["id"] == str(opportunity.id)
    assert response.json()["opportunityType"] == "business"

    foreign = User(id=uuid4(), email="foreign@example.test")
    foreign_client = build_client(foreign, repo, FakeMessageRepo([]))
    assert foreign_client.get(f"/opportunities/{opportunity.id}").status_code == 404


def test_detail_exposes_job_opportunity_type_for_mobile_templates() -> None:
    owner = User(id=uuid4(), email="owner@example.test")
    opportunity = make_opportunity(owner.id, OpportunityType.JOB)
    client = build_client(owner, FakeOpportunityRepo(opportunity), FakeMessageRepo([]))

    response = client.get(f"/opportunities/{opportunity.id}")

    assert response.status_code == 200
    assert response.json()["opportunityType"] == "job"


def test_message_page_is_bounded_and_keeps_chronological_metadata() -> None:
    owner = User(id=uuid4(), email="owner@example.test")
    opportunity = make_opportunity(owner.id)
    message_repo = FakeMessageRepo(make_messages(opportunity.id, 5))
    client = build_client(owner, FakeOpportunityRepo(opportunity), message_repo)

    response = client.get(
        "/messages/page",
        params={"opportunity_id": str(opportunity.id), "limit": 2, "offset": 2},
    )

    assert response.status_code == 200
    assert response.json()["total"] == 5
    assert response.json()["limit"] == 2
    assert response.json()["offset"] == 2
    assert [item["content"] for item in response.json()["items"]] == ["消息 2", "消息 3"]
    assert message_repo.calls == [(2, 2)]
    assert client.get(
        "/messages/page",
        params={"opportunity_id": str(opportunity.id), "limit": 201},
    ).status_code == 422


def test_message_routes_hide_foreign_history_and_bound_legacy_clients() -> None:
    owner = User(id=uuid4(), email="owner@example.test")
    foreign = User(id=uuid4(), email="foreign@example.test")
    opportunity = make_opportunity(owner.id)
    message_repo = FakeMessageRepo(make_messages(opportunity.id, 503))
    foreign_client = build_client(foreign, FakeOpportunityRepo(opportunity), message_repo)

    legacy = foreign_client.get("/messages", params={"opportunity_id": str(opportunity.id)})
    page = foreign_client.get("/messages/page", params={"opportunity_id": str(opportunity.id)})

    assert legacy.status_code == 200 and legacy.json() == []
    assert page.status_code == 200
    assert page.json() == {"items": [], "total": 0, "limit": 50, "offset": 0}
    assert message_repo.calls == []

    owner_client = build_client(owner, FakeOpportunityRepo(opportunity), message_repo)
    response = owner_client.get(
        "/messages", params={"opportunity_id": str(opportunity.id)}
    )
    assert response.status_code == 200
    assert len(response.json()) == 500
    assert response.json()[0]["content"] == "消息 3"
    assert response.json()[-1]["content"] == "消息 502"
    assert message_repo.calls == [(500, -1)]
