# Tasks for Migration to AWS

## Phase 1: Environment Setup & Infrastructure
## Phase 1: Environment Setup & Infrastructure
- [x] **Infrastructure as Code (Terraform)**:
  - [x] **Migrate Infrastructure**: Moved from `jyatesdotdev-api/infra` to a dedicated `jyatesdotdev-infra` repository.
  - [x] **Set up CI/CD**: Created GitHub Actions for automated deployment.

- [x] **Security Scanning**:
  - [x] Set up `tfsec` or `checkov` for IaC scanning.
  - [x] Configure `npm audit` and SAST (CodeQL) for application code.
  - [x] Configure `github/workflows/security.yml` to run scans on PR.
  - [x] Run local scanning with `tfsec`.
- [x] **Testing**:
  - [x] Validate Terraform configuration with `terraform validate`.
  - [x] Investigate to see if there is a testing framework for Terraform (Identified Terratest as the primary choice).


## Phase 2: Local Development Environment
- [x] **Docker Orchestration**:
  - [x] Create `docker-compose.yml` for local dependencies.
  - [x] Configure `amazon/dynamodb-local`.
  - [x] Configure `localstack` or SAM CLI for Lambda/API Gateway emulation.
  - [x] Create `localstack-init/01_init.sh` for resource initialization.
- [ ] **Shared Utilities**:

  - [x] Implement DynamoDB client with local/remote toggle.
  - [x] Implement ReCAPTCHA v3 verification service.
  - [x] Implement SES email service.

## Phase 3: Backend Development (Lambda)
- [ ] **Interaction Service**:
  - [x] Implement `GET/POST /api/v1/likes` (post likes) with ReCAPTCHA (consistent protection).
  - [x] Implement `GET/POST /api/v1/comments` with ReCAPTCHA and `bluemonday`.
  - [x] Implement `POST /api/v1/comments/:id/like` with ReCAPTCHA (consistent protection).
  - [x] Implement atomic increments/decrements for `likeCount` in DynamoDB.

  - [x] Implement optimized `userHasLiked` check for comments using `POST#<slug>#USER#<ipAddress>` query.
  - [x] **Admin Notification**: Implement logic to send an SES email to the admin when a new comment is submitted.
- [ ] **Contact Service**:
  - [x] Implement `POST /api/v1/contact` with ReCAPTCHA and SES.
- [ ] **Admin Service**:
  - [x] Implement Lambda Authorizer for Basic Auth.
  - [x] Implement `GET/PUT/DELETE /api/v1/admin/comments` for moderation.
- [ ] **Testing**:
  - [x] Write Go tests for all handlers (90% coverage target).
  - [x] Write integration tests against `dynamodb-local`.

## Phase 4: Frontend Development (SPA)
- [ ] **Vite Setup**:
  - [ ] Initialize React/TS project in `spa/`.
  - [ ] Configure Tailwind CSS v4 Alpha and Geist fonts.
- [ ] **UI Migration**:
  - [ ] Port `global.css` and custom `.prose` overrides.
  - [ ] Implement client-side theme switching (removing Next.js cookie dependency).
  - [ ] Port all shared components (`nav`, `footer`, `recaptcha-provider`).
  - [ ] Port `not-found.tsx` to a React Router catch-all `*` route for 404 handling.
- [ ] **MDX & Content**:
  - [ ] Set up `@mdx-js/rollup` for build-time MDX compilation (with `remark-gfm` and `sugar-high` support).
  - [ ] Port `mdx.tsx` components (replacing `next/link` and `next/image`).
  - [ ] Implement SEO metadata via `react-helmet-async` and `vite-plugin-ssr`.
  - [ ] **Static Asset Generation**: Implement build-time script to generate `sitemap.xml` (including the previously missing `/career` route), `robots.txt`, and `rss.xml`.
- [ ] **Features**:
  - [ ] Implement Blog index with client-side sorting, tag filtering, and pagination.
  - [ ] Implement Career, Projects, and Library pages.
  - [ ] Implement Contact Form and Post Interactions (Likes/Comments) with consistent reCAPTCHA usage.
  - [ ] **Admin Dashboard Migration**: 
    - [ ] Port the moderation UI to the SPA.
    - [ ] Update `PUT`/`DELETE` calls to include both `slug` and `commentId` for DynamoDB lookups.
- [ ] **Testing**:
  - [ ] Write Vitest/RTL tests for all components (80% coverage target).

## Phase 5: Data Migration & Validation
- [ ] **Data Migration**:
  - [ ] Create script to migrate existing Postgres comments/likes to DynamoDB.
- [ ] **Audit & Validation**:
  - [ ] Perform a full UI audit to ensure 1:1 design match.
  - [ ] Verify security of admin endpoints (both UI and API).
  - [ ] Validate SEO tags and Open Graph images.
  - [ ] Run full test suite and security scans.

## Phase 6: Deployment & Go-Live
- [ ] Deploy infrastructure via Terraform.
- [ ] Deploy backend Lambdas.
- [ ] Build and deploy SPA to S3.
- [ ] Perform DNS cutover (Route 53).
- [ ] Final production smoke test.
