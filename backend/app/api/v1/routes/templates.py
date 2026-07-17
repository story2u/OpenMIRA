from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, status

from app.api.deps import get_template_repo, require_admin, require_user
from app.application.dto import ReplyTemplateCreate, ReplyTemplateRead, ReplyTemplateUpdate
from app.application.mappers import to_reply_template_read
from app.infrastructure.db.models import ReplyTemplate
from app.infrastructure.db.repositories import ReplyTemplateRepository

router = APIRouter()


@router.get("", response_model=list[ReplyTemplateRead])
async def list_templates(
    _: object = Depends(require_user),
    repo: ReplyTemplateRepository = Depends(get_template_repo),
) -> list[ReplyTemplateRead]:
    templates = await repo.list(limit=200)
    return [to_reply_template_read(template) for template in templates]


@router.post("", response_model=ReplyTemplateRead)
async def create_template(
    payload: ReplyTemplateCreate,
    _: None = Depends(require_admin),
    repo: ReplyTemplateRepository = Depends(get_template_repo),
) -> ReplyTemplateRead:
    template = await repo.create(ReplyTemplate(**payload.model_dump()))
    return to_reply_template_read(template)


@router.patch("/{template_id}", response_model=ReplyTemplateRead)
async def update_template(
    template_id: UUID,
    payload: ReplyTemplateUpdate,
    _: None = Depends(require_admin),
    repo: ReplyTemplateRepository = Depends(get_template_repo),
) -> ReplyTemplateRead:
    template = await repo.get(template_id)
    if not template:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="template not found")
    for key, value in payload.model_dump(exclude_unset=True).items():
        setattr(template, key, value)
    return to_reply_template_read(await repo.save(template))
