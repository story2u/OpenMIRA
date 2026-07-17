import type { components, paths } from './openapi';

export type Dashboard = components['schemas']['DashboardRead'];
export type Opportunity = components['schemas']['OpportunityRead'];
export type OpportunityDetail = components['schemas']['OpportunityDetailRead'];
export type OpportunityStatusUpdate = components['schemas']['OpportunityStatusUpdate'];
export type DashboardQuery = NonNullable<
  paths['/api/v1/opportunities/dashboard']['get']['parameters']['query']
>;
export type OpportunityListQuery = NonNullable<
  paths['/api/v1/opportunities']['get']['parameters']['query']
>;
export type OpportunityDetailPath = NonNullable<
  paths['/api/v1/opportunities/{opportunity_id}']['get']['parameters']['path']
>;
