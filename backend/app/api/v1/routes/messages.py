from uuid import UUID

from fastapi import APIRouter, Depends, Query

from app.api.deps import get_message_repo, get_opportunity_repo, require_user
from app.application.dto import ChatMessageRead, MessagePageRead
from app.application.mappers import to_chat_message_read
from app.infrastructure.db.models import User
from app.infrastructure.db.repositories import MessageRepository, OpportunityRepository

router = APIRouter()
LEGACY_MESSAGE_LIMIT = 500


@router.get("", response_model=list[ChatMessageRead])
async def list_messages(
    opportunity_id: UUID,
    current_user: User = Depends(require_user),
    repo: MessageRepository = Depends(get_message_repo),
    opportunity_repo: OpportunityRepository = Depends(get_opportunity_repo),
) -> list[ChatMessageRead]:
    """兼容旧移动端的数组响应，但禁止无界读取。新客户端使用 `/page`。"""
    opportunity = await opportunity_repo.get(opportunity_id)
    if not opportunity or opportunity.owner_user_id != current_user.id:
        return []
    messages = await repo.list_recent_by_opportunity(
        opportunity_id,
        limit=LEGACY_MESSAGE_LIMIT,
    )
    return [to_chat_message_read(message) for message in messages]


@router.get("/page", response_model=MessagePageRead)
async def page_messages(
    opportunity_id: UUID,
    limit: int = Query(default=50, ge=1, le=200),
    offset: int = Query(default=0, ge=0),
    current_user: User = Depends(require_user),
    repo: MessageRepository = Depends(get_message_repo),
    opportunity_repo: OpportunityRepository = Depends(get_opportunity_repo),
) -> MessagePageRead:
    """按时间正序分页读取 owner 隔离的消息；未知资源返回空页以隐藏其存在性。"""
    opportunity = await opportunity_repo.get(opportunity_id)
    if not opportunity or opportunity.owner_user_id != current_user.id:
        return MessagePageRead(items=[], total=0, limit=limit, offset=offset)
    messages = await repo.list_by_opportunity(opportunity_id, limit=limit, offset=offset)
    total = await repo.count_by_opportunity(opportunity_id)
    return MessagePageRead(
        items=[to_chat_message_read(message) for message in messages],
        total=total,
        limit=limit,
        offset=offset,
    )
