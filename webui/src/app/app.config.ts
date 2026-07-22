import {
  ApplicationConfig,
  provideZoneChangeDetection,
  inject,
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
import { setContext } from "@apollo/client/link/context";
import { onError } from "@apollo/client/link/error";
import { graphqlEndpoint } from "../environments/environment";
import { TranslocoImportLoader } from "./i18n/transloco.loader";
import { routes } from "./app.routes";
import { API_KEY_HEADER, AuthService } from "./auth/auth.service";

export const appConfig: ApplicationConfig = {
  providers: [
    provideZoneChangeDetection({ eventCoalescing: true }),
    provideRouter(routes, withComponentInputBinding()),
    provideAnimationsAsync("animations"),
    provideHttpClient(withInterceptorsFromDi()),
    provideHttpClient(),
    provideApollo(() => {
      const httpLink = inject(HttpLink);
      const auth = inject(AuthService);

      // Attach the stored API key to every request. When no key is stored the
      // header is omitted, so a server with auth disabled works unchanged.
      const authLink = setContext((_, prevContext) => {
        const headers = (prevContext["headers"] ?? {}) as Record<
          string,
          string
        >;
        const apiKey = auth.getApiKey();
        return {
          headers: apiKey ? { ...headers, [API_KEY_HEADER]: apiKey } : headers,
        };
      });

      // On a 401 the credential is missing or rejected: flag it so the shell
      // can prompt for a key, rather than failing silently or with an opaque
      // network error.
      const errorLink = onError(({ networkError }) => {
        if (
          networkError &&
          "statusCode" in networkError &&
          networkError.statusCode === 401
        ) {
          auth.notifyAuthRequired();
        }
      });

      return {
        link: ApolloLink.from([
          authLink,
          errorLink,
          httpLink.create({ uri: graphqlEndpoint }),
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
