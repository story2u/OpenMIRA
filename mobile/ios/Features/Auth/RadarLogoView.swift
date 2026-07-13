import SwiftUI

/// 应用内品牌标识。图像来自与 AppIcon 相同的 1024px 母版。
struct RadarLogoView: View {
    var size: CGFloat = 72

    var body: some View {
        Image("BrandLogo")
            .resizable()
            .scaledToFit()
            .clipShape(RoundedRectangle(cornerRadius: size * 0.22, style: .continuous))
        .frame(width: size, height: size)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("商机雷达")
    }
}
