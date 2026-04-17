# Architectural Design for jyates.dev on AWS

## 1. High-Level Architecture
The application is split into a static frontend SPA and a serverless backend.

- **Frontend**: React/TypeScript/Vite application hosted on S3 and delivered via CloudFront.
- **Backend**: AWS Lambda functions behind API Gateway.
- **Database**: DynamoDB (Single Table Design) for storing all persistent state (likes, comments).
- **Email Service**: AWS SES for the contact form.

## 2. Frontend Design (SPA)

### 2.1 Framework and Tools
- **React (v18+)**: Component-based UI.
- **Vite**: Build tool and dev server.
- **Tailwind CSS (v4 Alpha)**: Styling (preserving current design).
- **Geist (Sans & Mono)**: Standardized fonts.
- **React Router**: Client-side routing.

### 2.2 MDX Integration
- **Build-time compilation**: A Vite plugin (e.g., `@mdx-js/rollup`) to import and render MDX files as React components, including `remark-gfm` for table support.
- **Data Loading**: Since `fs` is unavailable in the SPA, blog post metadata will be retrieved using Vite's `import.meta.glob` during the build process to generate a static registry.
- **Custom Components**: The `CustomMDX` configuration must map elements correctly. `next/link` will become React Router's `Link`, and `next/image` will become standard `<img>` tags.
- **Syntax Highlighting**: Ensure `sugar-high` functions properly with build-time MDX, retaining custom slugification logic from `mdx.tsx` and all associated CSS variables in `global.css`.

### 2.3 Routing
- `/`: Home
- `/blog`: Blog index
- `/blog/:slug`: Blog post view
- `/career`: Career summary (Static data from `src/data/career.ts`)
- `/projects`: Project list (Static data from `src/data/projects.ts`)
- `/library`: Library list (Static data from `src/data/library.ts`)
- `/contact`: Contact form
- `/admin`: Admin dashboard for comments

> **Note**: Career, Projects, and Library data are bundled as static TypeScript files within the SPA, matching the current implementation's efficiency and SEO benefits without requiring database lookups.

### 2.4 SEO and Metadata
As an SPA, dynamic SEO will be managed using:
- **Build-time Static Generation**: Using tools like `vite-plugin-ssr` or `react-snap` to pre-render routes into static HTML files for SEO.
- **Static Assets**: A build script will generate `sitemap.xml`, `robots.txt`, and `rss.xml` (which CloudFront will serve at `/rss`).
- **Dynamic Meta Tags**: Using `react-helmet-async` for client-side tag management.
- **Open Graph Image Handling**: The current `app/og/route.tsx` is a redirector to static images in `public/images/og/`. In the SPA, metadata can link directly to these static assets in S3, simplifying the architecture by removing the need for a dedicated redirect API.
- **Analytics & Observability**: 
  - Replace `@vercel/analytics` and `@vercel/speed-insights` with **AWS CloudWatch RUM**. 
  - This requires creating a CloudWatch RUM app monitor and embedding the generated snippet in the SPA entry point (`index.html` or `RootLayout` equivalent).
- **Theme-aware Metadata**: Replace the Next.js `cookies()` based dynamic metadata rendering with static rendering + client-side hydration for dark/light mode toggles.
- **Client-side Interactivity**: The blog index will implement client-side sorting, tag filtering, and pagination, replacing the current Next.js server-side logic. This will use React state and `useSearchParams` for a smooth, single-page experience.
- **Client-side Randomization**: Move the home page library sampling from the server to the client using a `useEffect` hook. This ensures that the "random" selection is fresh for each visitor without requiring a new build of the static site.

### 2.5 State Management
- Utilizing React Context for global providers (e.g., `ReCaptchaProvider`).
- Local component state for ephemeral UI state (e.g., like counts, comment form inputs).
- **Admin UI Updates**: The Admin UI must be updated to send both the `slug` and `commentId` for `PUT` and `DELETE` requests, as these are both required for efficient lookups in the DynamoDB Single Table Design.

## 3. Backend Design (Serverless)

### 3.1 API Gateway
- **Endpoint**: `/api/v1`
- **Authentication**: **Mandatory** Basic Authentication for all `/api/v1/admin/*` routes. 
  - **Implementation**: Lambda Authorizer that checks the `Authorization` header against a secret stored in AWS Systems Manager (SSM) Parameter Store or Secrets Manager.
- **Frontend Auth**: Basic Authentication for the `/admin` SPA route (implemented via CloudFront Function or Lambda@Edge to intercept requests and enforce credentials).
- **Subdomain Rewrite**: A CloudFront Function to handle `blog.jyates.dev` requests by rewriting the URI to prefix with `/blog`, mimicking the existing `next.config.js` rewrite behavior.
- **IP Extraction & Headers**: CloudFront must be configured to pass the `X-Forwarded-For` or `True-Client-IP` headers. The Lambda handlers must use `event.requestContext.http.sourceIp` (or the REST API equivalent) to ensure accurate IP-based toggling for likes.
- **CSP & Security**: CloudFront Response Headers Policy will set a strict CSP, replacing Vercel-specific entries with AWS endpoints (e.g., API Gateway, S3, CloudFront).

### 3.2 Lambda Functions
A set of Lambda functions written in Go to handle various API endpoints.
- **Interactions Lambda**:
  - `GET /api/v1/comments?slug=...`: Fetch approved comments.
  - `POST /api/v1/comments`: Submit a comment (ReCAPTCHA v3 verification + `bluemonday` sanitization + DynamoDB write).
  - `POST /api/v1/comments/:commentId/like`: Toggle a comment like (ReCAPTCHA v3 required).
  - `GET /api/v1/likes?slug=...`: Fetch post likes and user status.
  - `POST /api/v1/likes`: Toggle a post like (ReCAPTCHA v3 required - addresses current gap).
- **Contact Lambda**:
  - `POST /api/v1/contact`: Send email via SES (ReCAPTCHA v3 required).
- **Admin Lambda** (Requires Basic Auth Authorizer):
  - `GET /api/v1/admin/comments`: List comments for moderation (uses GSI1).
  - `PUT /api/v1/admin/comments/:commentId`: Update comment status (requires both `slug` and `commentId` in the body).
  - `DELETE /api/v1/admin/comments/:commentId`: Delete comment (requires both `slug` and `commentId` in the body).

### 3.3 DynamoDB Single Table Design
**Table Name**: `jyatesdotdev-state`

| PK | SK | Type | Attributes | GSI1PK | GSI1SK |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `POST#<slug>` | `METADATA` | PostMetadata | `commentCount`, `viewCount`, `likeCount` | - | - |
| `POST#<slug>` | `LIKE#<ipAddress>` | PostLike | `createdAt` | - | - |
| `POST#<slug>` | `COMMENT#<id>` | Comment | `content`, `authorName`, `authorEmail`, `ipAddress`, `status`, `createdAt`, `updatedAt`, `likeCount` | `STATUS#<status>` | `POST#<slug>#<timestamp>` |
| `POST#<slug>#USER#<ipAddress>` | `LIKE#COMMENT#<id>` | CommentLike | `createdAt` | - | - |
| `COMMENT#<id>` | `LIKE#<ipAddress>` | CommentLike | `createdAt` | - | - |

- **Access Pattern 1**: Get post likes and user status.
  - Query `PK = POST#<slug>` AND `SK begins_with(LIKE#)`.
- **Access Pattern 2**: Get approved comments for a post.
  - Query `GSI1PK = STATUS#approved` AND `GSI1SK begins_with(POST#<slug>)`.
- **Access Pattern 3**: Admin moderation (get all pending comments).
  - Query `GSI1PK = STATUS#pending` ORDER BY `GSI1SK`.
- **Access Pattern 4**: Toggle comment like.
  - 1. Get `PK = COMMENT#<id>` AND `SK = LIKE#<ipAddress>`.
  - 2. If exists:
    - Delete `PK = COMMENT#<id>` / `SK = LIKE#<ipAddress>`.
    - Delete `PK = POST#<slug>#USER#<ipAddress>` / `SK = LIKE#COMMENT#<id>`.
    - Atomic decrement `likeCount` on `PK = POST#<slug>` / `SK = COMMENT#<id>`.
  - 3. If not exists:
    - Create `PK = COMMENT#<id>` / `SK = LIKE#<ipAddress>`.
    - Create `PK = POST#<slug>#USER#<ipAddress>` / `SK = LIKE#COMMENT#<id>`.
    - Atomic increment `likeCount` on `PK = POST#<slug>` / `SK = COMMENT#<id>`.
- **Access Pattern 5**: Get all comments user liked on a post.
  - Query `PK = POST#<slug>#USER#<ipAddress>` AND `SK begins_with(LIKE#COMMENT#)`.

### 3.4 Local Development Environment
- **Docker-based Emulation**: 
  - `amazon/dynamodb-local` for database.
  - `localstack` or `SAM CLI` for API Gateway and Lambda emulation.
- **Docker Compose**: A unified `docker-compose.yml` to orchestrate all local backend dependencies.

## 4. Infrastructure (IaC)
- **Repository**: All infrastructure is managed in the dedicated `jyatesdotdev-infra` repository.
- **Organization**: Comprehensive Terraform configuration (`.tf`) files organized into the following modules:
  - `s3`: Static hosting bucket with website configuration and bucket policy.
  - `cloudfront`: Distribution with OAC for S3 access, custom error responses for SPA routing, and CloudFront Functions for Basic Auth on `/admin`.
  - `dynamodb`: Table definition with GSI and TTL (if needed).
  - `lambda`: Function definitions, code packaging (using S3 artifacts), and IAM roles with least-privilege policies.
  - `api_gateway`: REST API, resources, methods, integrations, and the Custom Authorizer.
  - `ses`: Domain/Email identity and verification records (Route 53).
- **State Backend**: Configuration for S3 backend with DynamoDB locking.
- **Security Headers**: CloudFront Response Headers Policy to inject CSP, X-Frame-Options, HSTS, and X-Content-Type-Options.
- **Modularization**: Each module should be independent and reusable where possible, with a clear separation of concerns.

## 5. Quality Assurance & Security

### 5.1 Testing Strategy
- **Frontend**: Vitest + React Testing Library. Minimum 80% coverage.
- **Backend**: Vitest + AWS SDK V3 Client Mocks. Minimum 90% coverage for handler logic.
- **Integration**: Local integration tests against `dynamodb-local`.
- **End-to-End**: Playwright or Cypress for core user journeys (likes, comments, contact).

### 5.2 Security Posture
- **API Security**: No unprotected admin endpoints. Unified authorizer pattern.
- **Data Protection**: Encryption at rest for DynamoDB (AWS-managed key).
- **IAM**: Execution roles restricted to specific DynamoDB items (using Leading Keys where possible) and SES identities.
- **Scanning**: Integration of `tfsec` and `npm audit` into the CI/CD pipeline.
- **Infrastructure**: S3 buckets must have public access blocked, and access should only be via CloudFront Origin Access Control (OAC).
