"""本地联调辅助：创建/复用演示用户，认领无主 seed 数据，打印可用 JWT。

仅用于本地开发环境（配合 scripts/seed_demo.py 与移动端 DEBUG 登录通道），
不属于生产路径；生产用户一律走 OAuth。
"""

import asyncio

from sqlmodel import col, select

from app.core.config import get_settings
from app.core.security import create_access_token
from app.infrastructure.db.models import Opportunity
from app.infrastructure.db.repositories import UserRepository
from app.infrastructure.db.session import AsyncSessionLocal

DEMO_EMAIL = "demo@local.dev"


async def main() -> None:
    settings = get_settings()
    async with AsyncSessionLocal() as session:
        user = await UserRepository(session).get_or_create_oauth_user(
            provider="dev-local",
            provider_subject="demo",
            email=DEMO_EMAIL,
            display_name="演示用户",
            avatar_url="",
        )
        result = await session.exec(
            select(Opportunity).where(col(Opportunity.owner_user_id).is_(None))
        )
        orphans = list(result.all())
        for opportunity in orphans:
            opportunity.owner_user_id = user.id
            session.add(opportunity)
        await session.commit()

        print(f"演示用户: {user.email} ({user.id})")
        print(f"认领无主商机: {len(orphans)} 条")
        print("--- JWT（粘贴到 app 的开发调试登录） ---")
        print(create_access_token(subject=user.id, settings=settings))


if __name__ == "__main__":
    asyncio.run(main())
