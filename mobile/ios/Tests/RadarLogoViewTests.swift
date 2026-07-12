import SwiftUI
import XCTest
@testable import OpportunityRadar

@MainActor
final class RadarLogoViewTests: XCTestCase {
    func testBrandLogoRendersAsNonEmptyImage() throws {
        let renderer = ImageRenderer(content: RadarLogoView(size: 72))
        renderer.scale = 2

        let image = try XCTUnwrap(renderer.uiImage)

        XCTAssertEqual(image.size.width, 72, accuracy: 0.5)
        XCTAssertEqual(image.size.height, 72, accuracy: 0.5)
        XCTAssertGreaterThan(try XCTUnwrap(image.pngData()).count, 1_000)
    }
}
