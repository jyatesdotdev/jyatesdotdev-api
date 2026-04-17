# Requirements for jyates.dev Migration to AWS

## 1. Project Overview
Migrate the existing Next.js (Vercel) application to a static-hosted SPA (React/Vite) on AWS (S3/CloudFront) with a serverless backend (Lambda/API Gateway) and a NoSQL database (DynamoDB).

## 2. Functional Requirements

### 2.1 Blog
- Render blog posts from MDX files.
- Support for syntax highlighting using **sugar-high**.
- Custom MDX components (e.g., custom links, rounded images) must be supported.
- Display post metadata (title, date, summary, tags).
- Relative dates (e.g., "2mo ago").
- Support for images in blog posts.

### 2.2 Post Interactions
- **Likes**: Users can like/unlike a post. Likes are tracked by IP address to prevent duplicate likes. **Must include reCAPTCHA v3 protection** (currently missing in Next.js version).
- **Comments**:
  - Users can submit comments (name, email, content).
  - Comments must be sanitized (DOMPurify).
  - Comments require approval by an admin before being displayed.
  - **Admin Notification**: The system must send an email notification to the admin via SES when a new comment is submitted and awaiting approval.
  - Users can like/unlike comments (tracked by IP).
  - ReCAPTCHA v3 protection on comment submission and comment likes.

### 2.3 Site Sections
- **Home**: Brief introduction and recent blog posts.
- **Career**: Work history (data-driven).
- **Projects**: Portfolio of work (data-driven).
- **Library**: List of books/resources (data-driven).
- **Contact**: Contact form with ReCAPTCHA v3, sending emails via AWS SES to the admin.

### 2.4 Admin Area
- Protected by Basic Authentication for both UI and API.
- List all comments (filtered by status: pending, approved, rejected).
- Approve or reject comments.
- Delete comments.
- **Security**: The current Next.js implementation has a critical gap where `/api/admin/*` routes are unprotected because `middleware.ts` only checks for `/admin/*` paths. The new architecture MUST protect ALL admin resources.

### 2.5 SEO & Metadata
- Dynamic titles and meta descriptions for all pages.
- Open Graph (OG) tags and Twitter Cards.
- Static OG images (managed in `public/images/og/`).
- RSS Feed (`/rss`) generated at build time.
- **Sitemap Consistency**: The new `sitemap.xml` must include the missing `/career` route to ensure all primary navigation items are indexed.
- Robots.txt generated at build time.

## 3. Technical Requirements

### 3.1 Frontend
- **Framework**: React 18+ with TypeScript.
- **Build Tool**: Vite.
- **Styling**: Tailwind CSS (matching the look and feel of current site, adhering to current `global.css` variables).
- **Icons/Fonts**: Geist Sans and Geist Mono.
- **MDX Support**: Build-time compilation of MDX to standard React components (replacing `next-mdx-remote`) with support for **remark-gfm** (tables, etc.).
- **Static Asset Generation**: Build-time generation of `sitemap.xml`, `robots.txt`, and `rss.xml` (aliased to `/rss` via CloudFront).
- **Theming**: Support for light and dark mode based on system preference (must replace Next.js `cookies()` based theming with pure client-side logic).
- **Client-side Interactivity**: Implement client-side pagination and tag filtering for the blog index, moving logic from the current Next.js server component to the SPA.
- **Analytics**: Replace Vercel Analytics and Speed Insights with AWS CloudWatch RUM or a similar privacy-focused, AWS-compatible analytics solution.
- **Component Replacements**: Remove dependencies on Next.js specifics like `next/link` and `next/image`, replacing them with React Router `<Link>` and standard `<img>` tags optimized for the SPA.

### 3.2 Backend
- **Compute**: AWS Lambda functions (Go).
- **API**: Amazon API Gateway.
- **Database**: Amazon DynamoDB (Single Table Design) to replace Vercel Postgres/Prisma.
- **Email**: Amazon SES for contact form.
- **Security**:
  - **Mandatory ReCAPTCHA v3** for all user-facing write operations (comments, comment likes, post likes, contact form). This addresses the current inconsistency where post likes are unprotected.
  - **Comprehensive Basic Auth** for ALL `/admin` UI routes and `/api/v1/admin/*` API routes (MUST address current security gap in Next.js middleware where `/api/admin/*` was unprotected).
  - **Content Sanitization**: Port `dompurify` logic (or equivalent Go library like `bluemonday`) to Lambda handlers for comment processing.
  - **IP Address Extraction**: Lambda handlers must reliably extract user IP addresses from `event.requestContext` or standard proxy headers (`X-Forwarded-For`) to support like-toggling and rate limiting.
  - **Security headers**: CloudFront Response Headers Policy to inject CSP (updated for AWS domains), X-Frame-Options, etc.

### 3.3 Hosting & Infrastructure
- **Frontend Hosting**: S3 Bucket (Static Website Hosting).
- **CDN**: CloudFront Distribution.
- **Domain/DNS**: Route 53.
- **Subdomain Routing**: Support `blog.jyates.dev` mapping to the `/blog` section of the SPA, mimicking the current Next.js `rewrites()` logic.
- **IaC**: Complete Terraform configuration files must be maintained in the `jyatesdotdev-infra` repository, structured with modular organization.
- **Terraform Coverage**: Must include S3, CloudFront, DynamoDB, Lambda, API Gateway (with authorizers), SES, IAM Roles/Policies, and CloudWatch Logs.
- **State Management**: Terraform state should be managed in a remote backend (S3/DynamoDB) for collaborative development.

### 3.4 Local Development & Testing
- **Docker**: Must utilize Docker for local testing to emulate the AWS backend environment.
  - `amazon/dynamodb-local` for database.
  - `lambci/lambda` or similar for function emulation.
- **Testing Coverage**: Mandatory unit and integration testing coverage for both the SPA (Vitest/RTL) and backend logic (Vitest/AWS SDK mocks).
- **Security Scanning**: 
  - Mandatory SAST scanning (e.g., CodeQL, SonarQube).
  - Mandatory IaC scanning (e.g., `tfsec`, `checkov`).
  - Mandatory dependency scanning (e.g., `npm audit`, Dependabot).
  - All scans must pass with zero critical or high-severity vulnerabilities before deployment.

## 4. UI/Design Requirements
- **Exact Match**: The new SPA must match the design, layout, and theming of the current Next.js site perfectly.
- **Responsiveness**: Fully responsive design (mobile, tablet, desktop).
- **Performance**: High Lighthouse scores for Performance, Accessibility, Best Practices, and SEO.
- **Accessibility**: Semantic HTML and ARIA labels where appropriate.
