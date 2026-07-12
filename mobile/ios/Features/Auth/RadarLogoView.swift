import SwiftUI

/// 应用内品牌标识。AppIcon 由系统管理，页面内使用独立的可渲染组件保持视觉一致。
struct RadarLogoView: View {
    var size: CGFloat = 72

    var body: some View {
        ZStack {
            RoundedRectangle(cornerRadius: size * 0.22, style: .continuous)
                .fill(
                    LinearGradient(
                        colors: [Color(red: 0.04, green: 0.51, blue: 1.0),
                                 Color(red: 0.03, green: 0.34, blue: 0.78)],
                        startPoint: .top,
                        endPoint: .bottom
                    )
                )

            Image(systemName: "dot.radiowaves.left.and.right")
                .resizable()
                .scaledToFit()
                .foregroundStyle(.white)
                .padding(size * 0.18)
        }
        .frame(width: size, height: size)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("商机雷达")
    }
}
