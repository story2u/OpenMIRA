"""好友申请状态流转端点：合法流转持久化 + SOP 推进，非法流转/私聊 409。

TestClient + dependency_overrides + 假仓储，无需数据库。
"""

from uuid import uuid4

from fastapi import FastAPI
from fastapi.testclient import TestClient

from app.api.deps import get_opportunity_or_404, get_opportunity_repo, require_user
from app.api.v1.routes import opportunities as opportunities_route
from app.infrastructure.db.models import Opportunity, User, utc_now
from app.domain.enums import IMChannel


class FakeOpportunityRepo:
    async def set_friend_request(self, opportunity: Opportunity, *, status: str) -> Opportunity:
        opportunity.friend_request_status = status
        if status == "pending":
            opportunity.sop_stage = "friend_requested"
        elif status == "accepted":
            opportunity.sop_stage = "ready_to_chat"
        opportunity.updated_at = utc_now()
        return opportunity


def make_opportunity(**overrides) -> Opportunity:
    defaults = dict(
        id=uuid4(),
        channel=IMChannel.TELEGRAM,
        conversation_id="conv-1",
        title="t",
        summary="s",
        source_type="group",
        friend_request_status="not_sent",
        sop_stage="contact_extracted",
    )
    defaults.update(overrides)
    return Opportunity(**defaults)


def build_client(opportunity: Opportunity) -> TestClient:
    app = FastAPI()
    app.include_router(opportunities_route.router, prefix="/opportunities")
    app.dependency_overrides[require_user] = lambda: User(id=uuid4(), email="u@example.test")
    app.dependency_overrides[get_opportunity_or_404] = lambda: opportunity
    app.dependency_overrides[get_opportunity_repo] = lambda: FakeOpportunityRepo()
    return TestClient(app)


def post(client: TestClient, opportunity: Opportunity, status: str):
    return client.post(f"/opportunities/{opportunity.id}/friend-request", json={"status": status})


def test_send_moves_not_sent_to_pending_and_advances_sop() -> None:
    opportunity = make_opportunity()
    resp = post(build_client(opportunity), opportunity, "pending")
    assert resp.status_code == 200
    body = resp.json()
    assert body["friendRequestStatus"] == "pending"
    assert body["sopStage"] == "friend_requested"


def test_confirm_accepted_unlocks_chat_stage() -> None:
    opportunity = make_opportunity(friend_request_status="pending", sop_stage="friend_requested")
    resp = post(build_client(opportunity), opportunity, "accepted")
    assert resp.status_code == 200
    body = resp.json()
    assert body["friendRequestStatus"] == "accepted"
    assert body["sopStage"] == "ready_to_chat"


def test_confirm_rejected_keeps_stage() -> None:
    opportunity = make_opportunity(friend_request_status="pending", sop_stage="friend_requested")
    resp = post(build_client(opportunity), opportunity, "rejected")
    assert resp.status_code == 200
    body = resp.json()
    assert body["friendRequestStatus"] == "rejected"
    assert body["sopStage"] == "friend_requested"


def test_retry_after_rejection() -> None:
    opportunity = make_opportunity(friend_request_status="rejected", sop_stage="friend_requested")
    resp = post(build_client(opportunity), opportunity, "not_sent")
    assert resp.status_code == 200
    assert resp.json()["friendRequestStatus"] == "not_sent"


def test_illegal_transition_rejected_with_409() -> None:
    opportunity = make_opportunity()  # not_sent
    resp = post(build_client(opportunity), opportunity, "accepted")
    assert resp.status_code == 409


def test_private_source_conflicts() -> None:
    opportunity = make_opportunity(source_type="private", friend_request_status="n/a")
    resp = post(build_client(opportunity), opportunity, "pending")
    assert resp.status_code == 409


def test_unknown_status_is_422() -> None:
    opportunity = make_opportunity()
    resp = post(build_client(opportunity), opportunity, "auto_accept")
    assert resp.status_code == 422
