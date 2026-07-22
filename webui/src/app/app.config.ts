import {
  APP_INITIALIZER,
  ApplicationConfig,
  inject,
  provideZoneChangeDetection,
} from "@angular/core";
import { provideRouter, withComponentInputBinding } from "@angular/router";

import { provideAnimationsAsync } from "@angular/platform-browser/animations/async";
import {
  provideHttpClient,
  withInterceptorsFromDi,
} from "@angular/common/http";
import { provideTransloco } from "@jsverse/transloco";
import { provideCharts, withDefaultRegisterables } from "ng2-charts";
import { provideApollo } from "apollo-angular";
import { HttpLink } from "apollo-angular/http";
import { ApolloLink, InMemoryCache } from "@apollo/client/core";
import { onError } from "@apollo/client/link/error";
import { graphqlEndpoint } from "../environments/environment";
import { TranslocoImportLoader } from "./i18n/transloco.loader";
import { routes } from "./app.routes";
import { AuthService } from "./auth/auth.service";

export const appConfig: ApplicationConfig = {
  providers: [
    provideZoneChangeDetection({ eventCoalescing: true }),
    provideRouter(routes, withComponentInputBinding()),
    {
      provide: APP_INITIALIZER,
      multi: true,
      useFactory: () => {
        const auth = inject(AuthService);
        return () => auth.bootstrap();
      },
    },
    provideAnimationsAsync("animations"),
    provideHttpClient(withInterceptorsFromDi()),
    provideApollo(() => {
      const httpLink = inject(HttpLink);
      const auth = inject(AuthService);

      // Session authentication is carried only by the HttpOnly cookie.

      // Route to login on a 401 rather than failing with an opaque network
      // error. Apollo can surface either Angular's `status` or `statusCode`.
      const errorLink = onError(({ networkError }) => {
        const status = networkError
          ? ((networkError as { status?: number }).status ??
            (networkError as { statusCode?: number }).statusCode)
          : undefined;
        if (status === 401) {
          auth.notifyAuthRequired();
        }
      });

      return {
        link: ApolloLink.from([
          errorLink,
          httpLink.create({ uri: graphqlEndpoint, withCredentials: true }),
        ]),
        cache: new InMemoryCache({
          typePolicies: {
            Query: {
              fields: {
                search: {
                  merge(
                    existing: Record<string, unknown>,
                    incoming: Record<string, unknown>,
                  ): Record<string, unknown> {
                    return { ...existing, ...incoming };
                  },
                },
              },
            },
          },
        }),
      };
    }),
    provideTransloco({
      config: {
        availableLangs: [
          {
            id: "ar",
            label: "العربية",
          },
          {
            id: "ca",
            label: "Català",
          },
          {
            id: "de",
            label: "Deutsch",
          },
          {
            id: "en",
            label: "English",
          },
          {
            id: "es",
            label: "Español",
          },
          {
            id: "fr",
            label: "Français",
          },
          {
            id: "hi",
            label: "हिन्दी",
          },
          {
            id: "ja",
            label: "日本語",
          },
          {
            id: "nl",
            label: "Nederlands",
          },
          {
            id: "pt",
            label: "Português",
          },
          {
            id: "ru",
            label: "Русский",
          },
          {
            id: "tr",
            label: "Türkçe",
          },
          {
            id: "uk",
            label: "Українська",
          },
          {
            id: "zh",
            label: "中文",
          },
        ],
        defaultLang: "en",
        fallbackLang: "en",
        missingHandler: {
          // It will use the first language set in the `fallbackLang` property
          useFallbackTranslation: true,
        },
        // Remove this option if your application doesn't support changing language in runtime.
        reRenderOnLangChange: true,
        prodMode: false,
      },
      loader: TranslocoImportLoader,
    }),
    provideCharts(withDefaultRegisterables()),
  ],
};
