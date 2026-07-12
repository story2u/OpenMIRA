"""Dashboard 端点的输入校验与 trust 边界映射（纯逻辑，无需数据库）。"""

from app.api.v1.routes.opportunities import DASHBOARD_SORTS, TRUST_LEVEL_RANGES


class TestTrustLevelRanges:
    def test_boundaries_match_web_sop_ts(self) -> None:
        # 与 frontend/lib/sop.ts trustLevel 严格一致
        assert TRUST_LEVEL_RANGES["trusted"] == (80, 100)
        assert TRUST_LEVEL_RANGES["unverified"] == (60, 79)
        assert TRUST_LEVEL_RANGES["suspicious"] == (40, 59)
        assert TRUST_LEVEL_RANGES["risky"] == (0, 39)

    def test_ranges_are_contiguous_and_cover_0_100(self) -> None:
        ordered = sorted(TRUST_LEVEL_RANGES.values())
        assert ordered[0][0] == 0
        assert ordered[-1][1] == 100
        for (_, high), (low_next, _) in zip(ordered, ordered[1:]):
            assert low_next == high + 1


class TestDashboardSorts:
    def test_supported_sorts(self) -> None:
        assert DASHBOARD_SORTS == {"newest", "oldest", "confidence", "trust"}
