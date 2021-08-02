pipeline {
    agent any
    stages {
        stage('env') {
            steps {
                sh 'printenv | sort'
                sh 'pwd'
                sh 'ls -l'
            }
        }
        stage('Build') {
            steps {
                sh 'make docker-build'
            }
        }
        stage('Unit Test') {
            steps {
                sh 'make test'
            }
        }
        stage('Docker Test') {
            steps {
                sh 'mkdir -p jenkins_logs'
                echo 'Running Docker Test. Will save the log as artifact (docker_test.log).'
                sh 'make docker-test > jenkins_logs/docker_test.log 2>&1'
            }
        }
        stage('System Test') {
            steps {
                echo 'Running System Test. Will save the log as artifact (system_test.log).'
                sh 'make system-test > jenkins_logs/system_test.log 2>&1'
            }
        }
        stage('Publish') {
            when {
                expression { return env.GIT_BRANCH == 'master' }
            }
            steps {
                sh 'make publish'
            }
        }
    }
    post {
        always {
            script {
                if (fileExists('jenkins_logs/docker_test.log')) {
                    echo 'Printing last 200 lines of docker test log.'
                    sh 'tail -n 200 jenkins_logs/docker_test.log'
                }
                if (fileExists('jenkins_logs/system_test.log')) {
                    echo 'Printing last 200 lines of system test log.'
                    sh 'tail -n 200 jenkins_logs/system_test.log'
                }
            }
            archiveArtifacts artifacts: 'jenkins_logs/*', allowEmptyArchive: true
        }
    }
}
