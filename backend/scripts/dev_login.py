"""本地联调辅助：创建/复用演示用户，认领无主 seed 数据，生成一次性展示的密码。

仅用于本地开发环境（配合 scripts/seed_demo.py 与正式邮箱密码登录端点），不属于生产路径。
"""

import asyncio
import secrets

from sqlmodel import col, select

from app.core.security import hash_password
from app.infrastructure.db.models import Opportunity
from app.infrastructure.db.repositories import UserRepository
from app.infrastructure.db.session import AsyncSessionLocal

DEMO_EMAIL = "demo@local.dev"


async def main() -> None:
    async with AsyncSessionLocal() as session:
        user = await UserRepository(session).get_or_create_oauth_user(
            provider="dev-local",
            provider_subject="demo",
            email=DEMO_EMAIL,
            display_name="演示用户",
            avatar_url="",
        )
        password = secrets.token_urlsafe(18)
        user.password_hash = hash_password(password)
        session.add(user)
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
        print("--- 本次本地联调密码（再次运行脚本会重置） ---")
        print(password)


if __name__ == "__main__":
    asyncio.run(main())
