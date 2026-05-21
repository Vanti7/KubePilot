pipeline {
    agent any

    environment {
        HARBOR         = 'harbor.10.0.60.107.nip.io'
        HARBOR_PROJECT = 'kubepilot'
        GITOPS_REPO    = 'gitea.10.0.60.107.nip.io/vanti/kubepilot-gitops.git'
    }

    stages {

        stage('Version') {
            steps {
                script {
                    def base = readFile('VERSION').trim()
                    switch (env.BRANCH_NAME) {
                        case 'dev':
                            env.IMAGE_TAG      = "v${base}-alpha.${env.BUILD_NUMBER}"
                            env.GITOPS_BRANCH  = 'dev'
                            env.VALUES_PATH    = 'envs/dev/values.yaml'
                            break
                        case 'staging':
                            env.IMAGE_TAG      = "v${base}-beta.${env.BUILD_NUMBER}"
                            env.GITOPS_BRANCH  = 'staging'
                            env.VALUES_PATH    = 'envs/staging/values.yaml'
                            break
                        default:
                            error("Branch '${env.BRANCH_NAME}' is not handled by this pipeline.")
                    }
                    echo "Tag: ${env.IMAGE_TAG}  →  gitops/${env.GITOPS_BRANCH}"
                }
            }
        }

        stage('Build') {
            parallel {
                stage('Backend') {
                    steps {
                        sh """
                            docker build \
                                -t ${env.HARBOR}/${env.HARBOR_PROJECT}/backend:${env.IMAGE_TAG} \
                                -f backend/Dockerfile \
                                backend/
                        """
                    }
                }
                stage('Frontend') {
                    steps {
                        sh """
                            docker build \
                                -t ${env.HARBOR}/${env.HARBOR_PROJECT}/frontend:${env.IMAGE_TAG} \
                                -f frontend/Dockerfile \
                                frontend/
                        """
                    }
                }
            }
        }

        stage('Push') {
            steps {
                withCredentials([usernamePassword(
                    credentialsId: 'harbor-creds',
                    usernameVariable: 'HARBOR_USER',
                    passwordVariable: 'HARBOR_PASS'
                )]) {
                    sh """
                        echo "\${HARBOR_PASS}" | docker login ${env.HARBOR} -u "\${HARBOR_USER}" --password-stdin
                        docker push ${env.HARBOR}/${env.HARBOR_PROJECT}/backend:${env.IMAGE_TAG}
                        docker push ${env.HARBOR}/${env.HARBOR_PROJECT}/frontend:${env.IMAGE_TAG}
                    """
                }
            }
        }

        stage('Update GitOps') {
            steps {
                withCredentials([usernamePassword(
                    credentialsId: 'gitea-creds',
                    usernameVariable: 'GITEA_USER',
                    passwordVariable: 'GITEA_PASS'
                )]) {
                    sh """
                        set -e
                        rm -rf gitops-tmp
                        git clone --branch ${env.GITOPS_BRANCH} \
                            http://\${GITEA_USER}:\${GITEA_PASS}@${env.GITOPS_REPO} \
                            gitops-tmp

                        cd gitops-tmp
                        sed -i 's|tag: ".*"|tag: "${env.IMAGE_TAG}"|g' ${env.VALUES_PATH}

                        git config user.email "jenkins@kubepilot.local"
                        git config user.name "Jenkins"
                        git add ${env.VALUES_PATH}
                        git diff --cached --quiet || git commit -m "chore(release): bump to ${env.IMAGE_TAG} [skip ci]"
                        git push

                        cd ..
                        rm -rf gitops-tmp
                    """
                }
            }
        }
    }

    post {
        always {
            sh """
                docker rmi ${env.HARBOR}/${env.HARBOR_PROJECT}/backend:${env.IMAGE_TAG} || true
                docker rmi ${env.HARBOR}/${env.HARBOR_PROJECT}/frontend:${env.IMAGE_TAG} || true
            """
        }
        success {
            echo "Deployed ${env.IMAGE_TAG} → ${env.GITOPS_BRANCH}"
        }
        failure {
            echo "Pipeline failed for ${env.IMAGE_TAG}"
        }
    }
}
