import HomeScreen from '../../src/features/home/HomeScreen';
import LegacyHomeScreen from '../../src/features/home/LegacyHomeScreen';
import { isMiraConciergeUiEnabled } from '../../src/config/miraConciergeFlag';

export default isMiraConciergeUiEnabled() ? HomeScreen : LegacyHomeScreen;
